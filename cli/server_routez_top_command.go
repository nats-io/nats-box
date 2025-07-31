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
	"fmt"
	glog "log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/choria-io/fisk"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type SrvRoutezTopCmd struct {
	expected    int
	readTimeout int
	account     string
	server      string
}

func configureServerRoutezTopCommand(srv *fisk.CmdClause) {
	c := &SrvRoutezTopCmd{}

	routezTop := srv.Command("routez:top", "Shows real-time route monitoring statistics").Action(c.routezTopAction)
	routezTop.Flag("read-timeout", "Read timeout in seconds").Default("5").IntVar(&c.readTimeout)
	routezTop.Flag("expected", "Expected number of servers").Default("3").IntVar(&c.expected)
	routezTop.Flag("account", "The NATS account").StringVar(&c.account)
	routezTop.Flag("filter-server", "Filter by specific NATS server name").StringVar(&c.server)
}

func (c *SrvRoutezTopCmd) routezTopAction(_ *fisk.ParseContext) error {
	nc, _, err := prepareHelper("", natsOpts()...)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c.clearScreen()
	go c.runRoutezTop(ctx, nc)

	select {
	case <-ctx.Done():
		c.clearScreen()
		return nil
	}
}

func (c *SrvRoutezTopCmd) runRoutezTop(ctx context.Context, nc *nats.Conn) {
	forceCh := make(chan struct{}, 0)
	go func() {
		input := bufio.NewScanner(os.Stdin)
		for input.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			forceCh <- struct{}{}
		}
	}()

	var (
		sys              = newRoutezSysClient(nc)
		fetchTimeout     = c.routezFetchTimeout(30 * time.Second)
		fetchExpected    = c.routezFetchExpected(c.expected)
		fetchReadTimeout = c.routezFetchReadTimeout(time.Duration(c.readTimeout) * time.Second)
	)

	prevRoutez := make(map[string]*server.RouteInfo)
	c.clearScreen()

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
		keys := make([]string, 0)
		routez := make(map[string]*server.RouteInfo)
		routezServers, err := sys.routezPing(fetchTimeout, fetchReadTimeout, fetchExpected)
		if err != nil {
			glog.Println(err)
			continue
		}
		for _, srv := range routezServers {
			for _, r := range srv.Routez.Routes {
				account := r.Account
				if len(account) == 0 {
					account = "_"
				}
				if c.account != "" && account != c.account {
					continue
				}
				if c.server != "" && srv.Server.Name != c.server {
					continue
				}
				key := fmt.Sprintf("%s:%s/r:%d",
					srv.Server.Name,
					account,
					r.Rid,
				)
				keys = append(keys, key)
				routez[key] = r
			}
		}
		sort.Strings(keys)

		c.clearScreen()
		header := func() {
			glog.Printf("%-30s  %-20s  %-15s  %-10s  %-10s %-10s  %-10s  %-10s  %-10s  %-10s  %-14s  %-14s  %-10s",
				"NAME",
				"REMOTE",
				"RTT",
				"SUBS",
				"PENDING",
				"MSGS_TO",
				"MSGS_FROM",
				"BYTES_TO",
				"BYTES_FROM",
				"InMsgs/Sec",
				"OutMsgs/Sec",
				"InBytes/Sec",
				"OutBytes/Sec",
			)
		}
		header()
		glog.Println(strings.Repeat("-", 190))

		var totalInMsgs, totalOutMsgs, totalInBytes, totalOutBytes, totalPending int64
		var totalInMsgsPerSec, totalOutMsgsPerSec, totalInBytesPerSec, totalOutBytesPerSec int64

		for _, key := range keys {
			var inMsgsPerSec, inBytesPerSec int64
			var outMsgsPerSec, outBytesPerSec int64

			r := routez[key]
			proutez, ok := prevRoutez[key]
			if ok {
				inMsgsPerSec = r.InMsgs - proutez.InMsgs
				inBytesPerSec = r.InBytes - proutez.InBytes
				outMsgsPerSec = r.OutMsgs - proutez.OutMsgs
				outBytesPerSec = r.OutBytes - proutez.OutBytes
			}

			totalInMsgs += r.InMsgs
			totalOutMsgs += r.OutMsgs
			totalInBytes += r.InBytes
			totalOutBytes += r.OutBytes
			totalPending += int64(r.Pending)
			totalInMsgsPerSec += inMsgsPerSec
			totalOutMsgsPerSec += outMsgsPerSec
			totalInBytesPerSec += inBytesPerSec
			totalOutBytesPerSec += outBytesPerSec

			glog.Printf("%-30s  %-20s  %-15s  %-10s  %-10s %-10s  %-10s  %-10s  %-10s  %-10s  %-14s  %-14s  %-10s",
				key,
				r.RemoteName,
				r.RTT,
				c.routezNsize(int64(r.NumSubs)),
				c.routezPsize(int64(r.Pending)),
				c.routezNsize(int64(r.InMsgs)),
				c.routezNsize(int64(r.OutMsgs)),
				c.routezPsize(int64(r.InBytes)),
				c.routezPsize(int64(r.OutBytes)),
				c.routezNsize(inMsgsPerSec),
				c.routezNsize(outMsgsPerSec),
				c.routezPsize(inBytesPerSec),
				c.routezPsize(outBytesPerSec),
			)
		}
		glog.Println(strings.Repeat("-", 190))
		header()
		glog.Printf("%-30s  %-20s  %-15s  %-10s  %-10s %-10s  %-10s  %-10s  %-10s  %-10s  %-14s  %-14s  %-10s",
			fmt.Sprintf("ROUTES: %d", len(keys)),
			"",
			"",
			"",
			c.routezPsize(totalPending),
			c.routezNsize(totalOutMsgs),
			c.routezNsize(totalInMsgs),
			c.routezPsize(totalOutBytes),
			c.routezPsize(totalInBytes),
			c.routezPsize(totalInMsgsPerSec),
			c.routezPsize(totalOutMsgsPerSec),
			c.routezPsize(totalInBytesPerSec),
			c.routezPsize(totalOutBytesPerSec),
		)
		glog.Println(time.Now().UTC())
		prevRoutez = routez
	}
}

func (c *SrvRoutezTopCmd) clearScreen() {
	fmt.Print("\033[2J\033[1;1H")
}

func (c *SrvRoutezTopCmd) routezPsize(s int64) string {
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

func (c *SrvRoutezTopCmd) routezNsize(s int64) string {
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

// Fetch options for routez
type routezFetchOpts struct {
	Timeout     time.Duration
	ReadTimeout time.Duration
	Expected    int
}

type routezFetchOpt func(*routezFetchOpts) error

func (c *SrvRoutezTopCmd) routezFetchTimeout(timeout time.Duration) routezFetchOpt {
	return func(opts *routezFetchOpts) error {
		if timeout <= 0 {
			return fmt.Errorf("timeout has to be greater than 0")
		}
		opts.Timeout = timeout
		return nil
	}
}

func (c *SrvRoutezTopCmd) routezFetchReadTimeout(timeout time.Duration) routezFetchOpt {
	return func(opts *routezFetchOpts) error {
		if timeout <= 0 {
			return fmt.Errorf("read timeout has to be greater than 0")
		}
		opts.ReadTimeout = timeout
		return nil
	}
}

func (c *SrvRoutezTopCmd) routezFetchExpected(expected int) routezFetchOpt {
	return func(opts *routezFetchOpts) error {
		if expected <= 0 {
			return fmt.Errorf("expected request count has to be greater than 0")
		}
		opts.Expected = expected
		return nil
	}
}

// System client for routez
type routezSysClient struct {
	nc *nats.Conn
}

func newRoutezSysClient(nc *nats.Conn) routezSysClient {
	return routezSysClient{nc: nc}
}

const srvRoutezSubj = "$SYS.REQ.SERVER.%s.ROUTEZ"

type routezResp struct {
	Server routezServerInfo `json:"server"`
	Routez *server.Routez   `json:"data"`
}

type routezServerInfo struct {
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

func (s *routezSysClient) routezPing(fopts ...routezFetchOpt) ([]*routezResp, error) {
	subj := fmt.Sprintf(srvRoutezSubj, "PING")
	payload, err := json.Marshal(nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.routezFetch(subj, payload, fopts...)
	if err != nil {
		return nil, err
	}
	srvRoutez := make([]*routezResp, 0, len(resp))
	for _, msg := range resp {
		var routezResp *routezResp
		if err := json.Unmarshal(msg.Data, &routezResp); err != nil {
			return nil, err
		}
		srvRoutez = append(srvRoutez, routezResp)
	}
	return srvRoutez, nil
}

func (s *routezSysClient) routezFetch(subject string, data []byte, opts ...routezFetchOpt) ([]*nats.Msg, error) {
	if subject == "" {
		return nil, fmt.Errorf("expected subject")
	}

	conn := s.nc
	reqOpts := &routezFetchOpts{}
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
