// Copyright 2020-2025 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	glog "log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/choria-io/fisk"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type SrvJszTopCmd struct {
	stream      string
	expected    int
	readTimeout int
}

func configureServerJszTopCommand(srv *fisk.CmdClause) {
	c := &SrvJszTopCmd{}

	jszTop := srv.Command("jsz:top", "Shows real-time JetStream monitoring statistics").Action(c.jszTopAction)
	jszTop.Flag("stream", "Select a single stream").StringVar(&c.stream)
	jszTop.Flag("read-timeout", "Read timeout in seconds").Default("5").IntVar(&c.readTimeout)
	jszTop.Flag("expected", "Expected number of servers").Default("3").IntVar(&c.expected)
}

func (c *SrvJszTopCmd) jszTopAction(_ *fisk.ParseContext) error {
	nc, _, err := prepareHelper("", natsOpts()...)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c.clearScreen()
	go c.runJszTop(ctx, nc)

	select {
	case <-ctx.Done():
		c.clearScreen()
		return nil
	}
}

func (c *SrvJszTopCmd) runJszTop(ctx context.Context, nc *nats.Conn) {
	forceCh := make(chan struct{}, 0)
	var stoggle atomic.Bool
	go func() {
		input := bufio.NewScanner(os.Stdin)
		for input.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			stoggle.Store(!stoggle.Load())
			forceCh <- struct{}{}
		}
	}()

	var (
		sys              = newSysClient(nc)
		fetchTimeout     = c.fetchTimeout(30 * time.Second)
		fetchExpected    = c.fetchExpected(c.expected)
		fetchReadTimeout = c.fetchReadTimeout(time.Duration(c.readTimeout) * time.Second)
	)

	type stats struct {
		msgsPerSecMax  int64
		bytesPerSecMax int64
	}
	statsServers := make(map[string]stats)
	prevServers := make(map[string]jszResp)
	prevVarz := make(map[string]*varzResp)
	prevStreams := make(map[string]*streamDetail)

	tick := time.NewTicker(1 * time.Second).C
	first := true
	for {
		if !first {
			select {
			case <-ctx.Done():
				return
			case <-tick:
			case <-forceCh:
			}
		} else {
			first = false
		}
		start := time.Now()
		jszServers, err := sys.jszPing(jszEventOptions{
			jszOptions: jszOptions{
				Streams:          true,
				RaftGroups:       true,
				StreamLeaderOnly: true,
			},
		}, fetchTimeout, fetchReadTimeout, fetchExpected)
		if err != nil {
			glog.Println(err)
			continue
		}

		serverNames := make([]string, 0)
		servers := make(map[string]jszResp)
		streams := make(map[string]*streamDetail)
		keys := make([]string, 0)
		for _, resp := range jszServers {
			jsz := resp.JSInfo
			serverInfo := resp.Server
			serverNames = append(serverNames, serverInfo.Name)
			servers[serverInfo.Name] = resp
			for _, acc := range jsz.AccountDetails {
				for _, stream := range acc.Streams {
					// Convert nats.StreamState to server.StreamState
					state := server.StreamState{}
					stateBytes, _ := json.Marshal(stream.State)
					json.Unmarshal(stateBytes, &state)

					sd := &streamDetail{
						ServerID:   serverInfo.ID,
						StreamName: stream.Name,
						Account:    acc.Name,
						AccountID:  acc.Id,
						RaftGroup:  stream.RaftGroup,
						State:      state,
					}
					key := fmt.Sprintf("%s:%s:%s",
						serverInfo.Name,
						acc.Id,
						stream.Name,
					)
					keys = append(keys, key)
					streams[key] = sd
				}
			}
		}
		sort.Strings(serverNames)
		sort.Strings(keys)

		varz := make(map[string]*varzResp)
		varzServers, err := sys.varzPing(fetchTimeout, fetchReadTimeout, fetchExpected)
		if err != nil {
			glog.Println(err)
			continue
		}
		for _, srv := range varzServers {
			varz[srv.Server.Name] = srv
		}

		c.clearScreen()
		glog.Println("Servers :", len(servers))
		glog.Println("Streams :", len(streams))
		glog.Printf("JSZ took: %.3fs", time.Since(start).Seconds())
		glog.Println()

		showStreams := stoggle.Load()
		if showStreams {
			glog.Printf("%-50s  %-10s  %-10s  %-10s  %-10s  %-10s  %-10s  %-10s  %-10s",
				"NAME",
				"MSGS",
				"BYTES",
				"FIRST",
				"LAST",
				"DELETED",
				"SUBJECTS",
				"MSGS/SEC",
				"BYTES/SEC",
			)

			glog.Println(strings.Repeat("-", 150))
			var totalMsgsPerSec uint64
			var totalBytesPerSec uint64
			for _, key := range keys {
				var msgsPerSec, bytesPerSec uint64

				stream := streams[key]
				prevStream, ok := prevStreams[key]
				if ok {
					msgsPerSec = stream.State.LastSeq - prevStream.State.LastSeq
					bytesPerSec = stream.State.Bytes - prevStream.State.Bytes

					if bytesPerSec < 0 {
						bytesPerSec = 0
					}
					totalMsgsPerSec += msgsPerSec
					totalBytesPerSec += bytesPerSec
				}
				glog.Printf("%-50s  %-10s  %-10s  %-10d  %-10d  %-10d  %-10d  %-10s  %-10s",
					key,
					c.nsize(int64(stream.State.Msgs)),
					c.psize(int64(stream.State.Bytes)),
					stream.State.FirstSeq,
					stream.State.LastSeq,
					stream.State.NumDeleted,
					stream.State.NumSubjects,
					c.psize(int64(msgsPerSec)),
					c.psize(int64(bytesPerSec)),
				)
			}
			glog.Println(strings.Repeat("=", 150))
			glog.Printf("%-50s  %-10s  %-10s  %-10s  %-10s  %-10s  %-10s  %-10s  %-10s",
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				c.psize(int64(totalMsgsPerSec)),
				c.psize(int64(totalBytesPerSec)),
			)

			prevStreams = streams
		}

		var (
			totalMsgs             int64
			totalBytes            int64
			totalJSInBytes        int64
			totalJSOutBytes       int64
			totalJSInBytesPerSec  int64
			totalJSOutBytesPerSec int64

			totalMsgsPerSec      int64
			totalBytesPerSec     int64
			totalDelivered       int64
			totalDeliveredPerSec int64

			totalInMsgs         int64
			totalInBytes        int64
			totalInMsgsPerSec   int64
			totalInBytesPerSec  int64
			totalOutMsgs        int64
			totalOutBytes       int64
			totalOutMsgsPerSec  int64
			totalOutBytesPerSec int64

			totalCPU   int64
			totalMem   int64
			totalConns int64
		)
		for _, srvName := range serverNames {
			var msgsPerSec, bytesPerSec int64
			var msgsPerSecMax, bytesPerSecMax int64
			var deliveredMsgsPerSec int64

			srv := servers[srvName]
			_, ok := prevServers[srvName]
			if ok {
				if sts, ok := statsServers[srvName]; ok {
					msgsPerSecMax = int64(sts.msgsPerSecMax)
					bytesPerSecMax = int64(sts.bytesPerSecMax)
					if msgsPerSec > msgsPerSecMax {
						sts.msgsPerSecMax = msgsPerSec
					}
					if bytesPerSec > bytesPerSecMax {
						sts.bytesPerSecMax = bytesPerSec
					}
					msgsPerSecMax = sts.msgsPerSecMax
					bytesPerSecMax = sts.bytesPerSecMax
					statsServers[srvName] = sts
				} else {
					msgsPerSecMax = msgsPerSec
					bytesPerSecMax = bytesPerSec
					statsServers[srvName] = stats{
						msgsPerSecMax:  msgsPerSecMax,
						bytesPerSecMax: bytesPerSecMax,
					}
				}

				totalBytes += int64(srv.JSInfo.Bytes)
				totalBytesPerSec += int64(bytesPerSec)
			}

			var inMsgsPerSec, inBytesPerSec, inJSBytesPerSec int64
			var outMsgsPerSec, outBytesPerSec, outJSBytesPerSec int64
			vrz, ok := varz[srvName]
			if !ok {
				continue
			}
			pvrz, ok := prevVarz[srvName]
			if ok {
				inMsgsPerSec = vrz.Varz.InMsgs - pvrz.Varz.InMsgs
				inBytesPerSec = vrz.Varz.InBytes - pvrz.Varz.InBytes
				outMsgsPerSec = vrz.Varz.OutMsgs - pvrz.Varz.OutMsgs
				outBytesPerSec = vrz.Varz.OutBytes - pvrz.Varz.OutBytes
			}
			totalMsgs += vrz.Varz.InMsgs
			totalDelivered += vrz.Varz.OutMsgs
			totalJSInBytes += vrz.Varz.InBytes
			totalJSOutBytes += vrz.Varz.OutBytes

			totalInMsgs += vrz.Varz.InMsgs
			totalOutMsgs += vrz.Varz.OutMsgs
			totalInBytes += vrz.Varz.InBytes
			totalOutBytes += vrz.Varz.OutBytes
			totalInMsgsPerSec += inMsgsPerSec
			totalInBytesPerSec += inBytesPerSec
			totalOutMsgsPerSec += outMsgsPerSec
			totalOutBytesPerSec += outBytesPerSec
			totalMsgsPerSec += msgsPerSec
			totalDeliveredPerSec += deliveredMsgsPerSec
			totalJSInBytesPerSec += inJSBytesPerSec
			totalJSOutBytesPerSec += outJSBytesPerSec

			bytesPerSec = inJSBytesPerSec

			scStats := &server.SlowConsumersStats{}
			if vrz.Varz.SlowConsumersStats != nil {
				scStats = vrz.Varz.SlowConsumersStats
			}

			totalCPU += int64(vrz.Varz.CPU)
			totalMem += int64(vrz.Varz.Mem)
			totalConns += int64(vrz.Varz.Connections)

			if !showStreams {
				glog.Printf("● %-15s CPU: %-6d  Memory: %-6s    Conns: %-6s  SlowConsumers: (c=%d, r=%d, l=%d, g=%d)",
					srvName,
					int64(vrz.Varz.CPU),
					c.psize(int64(vrz.Varz.Mem)),
					c.nsize(int64(vrz.Varz.Connections)),
					scStats.Clients, scStats.Routes, scStats.Leafs, scStats.Gateways,
				)
				jsStats := ""
				if vrz.Varz.JetStream.Stats != nil {
					jsStats = fmt.Sprintf("JS Memory: %s Store: %s",
						c.psize(int64(vrz.Varz.JetStream.Stats.Memory)),
						c.psize(int64(vrz.Varz.JetStream.Stats.Store)))
				}
				glog.Printf("%-15s  %s",
					"       JS:",
					jsStats,
				)
				glog.Printf("%-15s      %-6s          %-6s            %-6s             %-6s",
					"       In:",
					c.nsize(int64(vrz.Varz.InMsgs)),
					c.psize(int64(vrz.Varz.InBytes)),
					c.nsize(int64(inMsgsPerSec)),
					c.psize(int64(inBytesPerSec)),
				)
				glog.Printf("%-15s      %-6s          %-6s            %-6s             %-6s",
					"       Out:",
					c.nsize(int64(vrz.Varz.OutMsgs)),
					c.psize(int64(vrz.Varz.OutBytes)),
					c.nsize(int64(outMsgsPerSec)),
					c.psize(int64(outBytesPerSec)),
				)
				glog.Println(strings.Repeat("-", 122))
			}
		}

		if !showStreams {
			glog.Println("      ", strings.Repeat("=", 115))
			glog.Printf("%-15s  CPU: %-6d    Mem: %-6s     Conns: %-6d             %-6s",
				"       NATS:",
				totalCPU,
				c.psize(totalMem),
				totalConns,
				"",
			)
			glog.Printf("%-15s Total Msgs: %-6s  Total Bytes: %-6s",
				"       NATS:",
				c.nsize(totalMsgs),
				c.psize(totalJSInBytes),
			)
			glog.Printf("%-15s      %-6s          %-6s            %-6s             %-6s      %-6s",
				"       In:",
				c.nsize(totalInMsgs),
				c.psize(totalInBytes),
				c.nsize(totalInMsgsPerSec),
				c.psize(totalInBytesPerSec),
				c.psizeBps(totalInBytesPerSec),
			)
			glog.Printf("%-15s      %-6s          %-6s            %-6s             %-6s      %-6s",
				"       Out:",
				c.nsize(totalInMsgs),
				c.psize(totalInBytes),
				c.nsize(totalInMsgsPerSec),
				c.psize(totalInBytesPerSec),
				c.psizeBps(totalInBytesPerSec),
			)
		}

		prevServers = servers
		prevVarz = varz
	}
}

func (c *SrvJszTopCmd) clearScreen() {
	fmt.Print("\033[2J\033[1;1H")
}

func (c *SrvJszTopCmd) psize(s int64) string {
	var nprefix string
	if s < 0 {
		return "↓"
	}
	size := float64(s)

	if size < 1024 {
		return fmt.Sprintf("%s%.0f", nprefix, size)
	} else if size < (1024 * 1024) {
		return fmt.Sprintf("%s%.1fK", nprefix, size/1024)
	} else if size < (1024 * 1024 * 1024) {
		return fmt.Sprintf("%s%.1fM", nprefix, size/1024/1024)
	} else if size < (1024 * 1024 * 1024 * 1024) {
		return fmt.Sprintf("%s%.1fG", nprefix, size/1024/1024/1024)
	} else if size >= (1024 * 1024 * 1024 * 1024) {
		return fmt.Sprintf("%s%.1fT", nprefix, size/1024/1024/1024/1024)
	} else {
		return "NA"
	}
}

func (c *SrvJszTopCmd) psizeBps(s int64) string {
	var nprefix string
	if s < 0 {
		return "↓"
	}
	size := float64(s) * 8

	if size < 1024 {
		return fmt.Sprintf("%s%.0f", nprefix, size)
	} else if size < (1024 * 1024) {
		return fmt.Sprintf("%s%.1fKbps", nprefix, size/1024)
	} else if size < (1024 * 1024 * 1024) {
		return fmt.Sprintf("%s%.1fMbps", nprefix, size/1024/1024)
	} else if size < (1024 * 1024 * 1024 * 1024) {
		return fmt.Sprintf("%s%.1fGbps", nprefix, size/1024/1024/1024)
	} else if size >= (1024 * 1024 * 1024 * 1024) {
		return fmt.Sprintf("%s%.1fTbps", nprefix, size/1024/1024/1024/1024)
	} else {
		return "NA"
	}
}

const k = 1000
const m = k * 1000
const b = m * 1000
const t = b * 1000

func (c *SrvJszTopCmd) nsize(s int64) string {
	if s < 0 {
		return ""
	}
	size := float64(s)

	switch {
	case size < k:
		return fmt.Sprintf("%.0f", size)
	case size < m:
		return fmt.Sprintf("%.1fK", size/k)
	case size < b:
		return fmt.Sprintf("%.1fM", size/m)
	case size < t:
		return fmt.Sprintf("%.1fB", size/b)
	default:
		return fmt.Sprintf("%.1fT", size/t)
	}
}

// Fetch options
type fetchOpts struct {
	Timeout     time.Duration
	ReadTimeout time.Duration
	Expected    int
}

type fetchOpt func(*fetchOpts) error

func (c *SrvJszTopCmd) fetchTimeout(timeout time.Duration) fetchOpt {
	return func(opts *fetchOpts) error {
		if timeout <= 0 {
			return fmt.Errorf("timeout has to be greater than 0")
		}
		opts.Timeout = timeout
		return nil
	}
}

func (c *SrvJszTopCmd) fetchReadTimeout(timeout time.Duration) fetchOpt {
	return func(opts *fetchOpts) error {
		if timeout <= 0 {
			return fmt.Errorf("read timeout has to be greater than 0")
		}
		opts.ReadTimeout = timeout
		return nil
	}
}

func (c *SrvJszTopCmd) fetchExpected(expected int) fetchOpt {
	return func(opts *fetchOpts) error {
		if expected <= 0 {
			return fmt.Errorf("expected request count has to be greater than 0")
		}
		opts.Expected = expected
		return nil
	}
}

// System client and supporting types
type sysClient struct {
	nc *nats.Conn
}

func newSysClient(nc *nats.Conn) sysClient {
	return sysClient{nc: nc}
}

const (
	srvJszSubj  = "$SYS.REQ.SERVER.%s.JSZ"
	srvVarzSubj = "$SYS.REQ.SERVER.%s.VARZ"
)

type jszResp struct {
	Server serverInfo `json:"server"`
	JSInfo jsInfo     `json:"data"`
}

type varzResp struct {
	Server serverInfo   `json:"server"`
	Varz   *server.Varz `json:"data"`
}

type serverInfo struct {
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	ID        string    `json:"id"`
	Cluster   string    `json:"cluster,omitempty"`
	Domain    string    `json:"domain,omitempty"`
	Version   string    `json:"ver"`
	Tags      []string  `json:"tags,omitempty"`
	Seq       uint64    `json:"seq"`
	JetStream bool      `json:"jetstream"`
	Time      time.Time `json:"time"`
}

type jsInfo struct {
	ID       string          `json:"server_id"`
	Now      time.Time       `json:"now"`
	Disabled bool            `json:"disabled,omitempty"`
	Config   jetStreamConfig `json:"config,omitempty"`
	jetStreamStats
	Streams   int              `json:"streams"`
	Consumers int              `json:"consumers"`
	Messages  uint64           `json:"messages"`
	Bytes     uint64           `json:"bytes"`
	Meta      *metaClusterInfo `json:"meta_cluster,omitempty"`

	AccountDetails []*accountDetail `json:"account_details,omitempty"`
}

type accountDetail struct {
	Name string `json:"name"`
	Id   string `json:"id"`
	jetStreamStats
	Streams []streamDetail2 `json:"stream_detail,omitempty"`
}

type streamDetail2 struct {
	Name               string                   `json:"name"`
	Created            time.Time                `json:"created"`
	Cluster            *nats.ClusterInfo        `json:"cluster,omitempty"`
	Config             *nats.StreamConfig       `json:"config,omitempty"`
	State              nats.StreamState         `json:"state,omitempty"`
	Consumer           []*nats.ConsumerInfo     `json:"consumer_detail,omitempty"`
	Mirror             *nats.StreamSourceInfo   `json:"mirror,omitempty"`
	Sources            []*nats.StreamSourceInfo `json:"sources,omitempty"`
	RaftGroup          string                   `json:"stream_raft_group,omitempty"`
	ConsumerRaftGroups []*raftGroupDetail       `json:"consumer_raft_groups,omitempty"`
}

type raftGroupDetail struct {
	Name      string `json:"name"`
	RaftGroup string `json:"raft_group,omitempty"`
}

type jszEventOptions struct {
	jszOptions
	eventFilterOptions
}

type jszOptions struct {
	Account          string `json:"account,omitempty"`
	Accounts         bool   `json:"accounts,omitempty"`
	Streams          bool   `json:"streams,omitempty"`
	Consumer         bool   `json:"consumer,omitempty"`
	Config           bool   `json:"config,omitempty"`
	LeaderOnly       bool   `json:"leader_only,omitempty"`
	Offset           int    `json:"offset,omitempty"`
	Limit            int    `json:"limit,omitempty"`
	RaftGroups       bool   `json:"raft,omitempty"`
	StreamLeaderOnly bool   `json:"stream_leader_only,omitempty"`
}

type jetStreamStats struct {
	Memory         uint64            `json:"memory"`
	Store          uint64            `json:"storage"`
	ReservedMemory uint64            `json:"reserved_memory"`
	ReservedStore  uint64            `json:"reserved_storage"`
	Accounts       int               `json:"accounts"`
	HAAssets       int               `json:"ha_assets"`
	API            jetStreamAPIStats `json:"api"`
}

type jetStreamConfig struct {
	MaxMemory  int64  `json:"max_memory"`
	MaxStore   int64  `json:"max_storage"`
	StoreDir   string `json:"store_dir,omitempty"`
	Domain     string `json:"domain,omitempty"`
	CompressOK bool   `json:"compress_ok,omitempty"`
	UniqueTag  string `json:"unique_tag,omitempty"`
}

type jetStreamAPIStats struct {
	Total    uint64 `json:"total"`
	Errors   uint64 `json:"errors"`
	Inflight uint64 `json:"inflight,omitempty"`
}

type metaClusterInfo struct {
	Name     string      `json:"name,omitempty"`
	Leader   string      `json:"leader,omitempty"`
	Peer     string      `json:"peer,omitempty"`
	Replicas []*peerInfo `json:"replicas,omitempty"`
	Size     int         `json:"cluster_size"`
}

type peerInfo struct {
	Name    string        `json:"name"`
	Current bool          `json:"current"`
	Offline bool          `json:"offline,omitempty"`
	Active  time.Duration `json:"active"`
	Lag     uint64        `json:"lag,omitempty"`
	Peer    string        `json:"peer"`
}

type eventFilterOptions struct {
	Name    string   `json:"server_name,omitempty"`
	Cluster string   `json:"cluster,omitempty"`
	Host    string   `json:"host,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Domain  string   `json:"domain,omitempty"`
}

func (s *sysClient) jszPing(opts jszEventOptions, fopts ...fetchOpt) ([]jszResp, error) {
	subj := fmt.Sprintf(srvJszSubj, "PING")
	payload, err := json.Marshal(opts)
	if err != nil {
		return nil, err
	}
	resp, err := s.fetch(subj, payload, fopts...)
	if err != nil {
		return nil, err
	}
	srvJsz := make([]jszResp, 0, len(resp))
	for _, msg := range resp {
		var jszResp jszResp
		if err := json.Unmarshal(msg.Data, &jszResp); err != nil {
			return nil, err
		}
		srvJsz = append(srvJsz, jszResp)
	}
	return srvJsz, nil
}

func (s *sysClient) varzPing(fopts ...fetchOpt) ([]*varzResp, error) {
	subj := fmt.Sprintf(srvVarzSubj, "PING")
	payload, err := json.Marshal(nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.fetch(subj, payload, fopts...)
	if err != nil {
		return nil, err
	}
	srvVarz := make([]*varzResp, 0, len(resp))
	for _, msg := range resp {
		var varzResp *varzResp
		if err := json.Unmarshal(msg.Data, &varzResp); err != nil {
			return nil, err
		}
		srvVarz = append(srvVarz, varzResp)
	}
	return srvVarz, nil
}

func (s *sysClient) fetch(subject string, data []byte, opts ...fetchOpt) ([]*nats.Msg, error) {
	if subject == "" {
		return nil, errors.New("expected subject")
	}

	conn := s.nc
	reqOpts := &fetchOpts{}
	for _, opt := range opts {
		if err := opt(reqOpts); err != nil {
			return nil, err
		}
	}

	inbox := nats.NewInbox()
	res := make([]*nats.Msg, 0)
	msgsChan := make(chan *nats.Msg, 100)

	readTimer := time.NewTimer(reqOpts.ReadTimeout)
	sub, err := conn.Subscribe(inbox, func(msg *nats.Msg) {
		readTimer.Reset(reqOpts.ReadTimeout)
		msgsChan <- msg
	})
	if err != nil {
		return nil, err
	}
	defer sub.Unsubscribe()

	if err := conn.PublishRequest(subject, inbox, data); err != nil {
		return nil, err
	}

	for {
		select {
		case msg := <-msgsChan:
			if msg.Header.Get("Status") == "503" {
				return nil, fmt.Errorf("server request on subject %q failed", subject)
			}
			res = append(res, msg)
			if reqOpts.Expected != -1 && len(res) == reqOpts.Expected {
				return res, nil
			}
		case <-readTimer.C:
			return res, nil
		case <-time.After(reqOpts.Timeout):
			return res, nil
		}
	}
}
