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
	"crypto/rand"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
	"github.com/choria-io/fisk"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nuid"
	histwriter "github.com/tylertreat/hdrhistogram-writer"
)

type perfCmd struct {
	targetPubRate int
	msgSize       int
	testDuration  time.Duration
	histFile      string
	numPubs       int
	subject       string
	minDelay      int
	inflight      int
	stall         int
}

func configurePerfCommand(app commandHost) {
	c := &perfCmd{}

	perf := app.Command("stream:perf", "Perform perf tests on a NATS JetStream subject").Action(c.perfAction)
	perf.Flag("delay", "Minimum delay in between publishes (ns)").Default("1").IntVar(&c.minDelay)
	perf.Flag("size", "Message size").Default("28").IntVar(&c.msgSize)
	perf.Flag("rate", "Rate of messages per second").Default("1000").IntVar(&c.targetPubRate)
	perf.Flag("duration", "Test duration").Default("1s").DurationVar(&c.testDuration)
	perf.Flag("histogram", "Output file to store the histogram in").StringVar(&c.histFile)
	perf.Flag("subject", "Stream subject").Default("foo").StringVar(&c.subject)
	perf.Flag("inflight", "Max Inflight Acks").Default("-1").IntVar(&c.inflight)
	perf.Flag("stall", "Stall time when there are too many inflight acks").Default("0").IntVar(&c.stall)
}

func init() {
	registerCommand("perf", 11, configurePerfCommand)
}

func (c *perfCmd) perfAction(_ *fisk.ParseContext) error {
	start := time.Now()
	c.numPubs = int(c.testDuration/time.Second) * c.targetPubRate

	if c.msgSize < 0 {
		return fmt.Errorf("message Payload Size must be at least %d bytes", 0)
	}

	c1, err := newNatsConn("", natsOpts()...)
	if err != nil {
		return fmt.Errorf("first connection failed: %v", err)
	}

	c2, err := newNatsConn("", natsOpts()...)
	if err != nil {
		return fmt.Errorf("first connection failed: %v", err)
	}

	// reset to not use the stored conn or context
	opts().Conn = nil

	// Do some quick RTT calculations
	log.Println("==============================")
	rtt, err := c1.RTT()
	if err != nil {
		return fmt.Errorf("could not determine RTT for first connection: %v", err)
	}
	log.Printf("Pub Server RTT : %v\n", c.fmtDur(rtt))

	// Duration tracking
	durations := make([]time.Duration, 0, c.numPubs)

	// Wait for all messages to be received.
	var wg sync.WaitGroup
	wg.Add(1)

	// Stream subject
	subject := c.subject

	inbox := fmt.Sprintf("_INBOX.%s", nuid.Next())

	// Count the messages.
	received := 0

	// Keep max N inflight
	maxInflight := int64(c.inflight)
	inflight := int64(0)
	var mu sync.Mutex
	cond := sync.NewCond(&mu)

	// Async Subscriber (Runs in its own Goroutine)
	i := 0
	var firstAckTime time.Time
	_, err = c2.Subscribe(fmt.Sprintf("%s.*", inbox), func(msg *nats.Msg) {
		i++
		if i == 1 {
			firstAckTime = time.Now()
		}
		st := strings.TrimPrefix(msg.Subject, inbox+".")
		sendTime, _ := strconv.Atoi(st)
		durations = append(durations, time.Duration(time.Now().UnixNano()-int64(sendTime)))
		received++

		if maxInflight > 0 {
			atomic.AddInt64(&inflight, -1)
			cond.Signal()
		}
		if received >= c.numPubs {
			wg.Done()
		}
		if received%10000 == 0 {
			// fmt.Printf("#")
		}
	})
	if err != nil {
		return fmt.Errorf("subscribing on second connection failed: %v", err)
	}
	c2.Flush()

	// wait for routes to be established so we get every message
	err = c.waitForRoute(c1, c1)
	if err != nil {
		return err
	}

	log.Printf("Server Name    : %v\n", c1.ConnectedServerName())
	log.Printf("Message Payload: %v\n", c.byteSize(c.msgSize))
	log.Printf("Target Duration: %v\n", c.testDuration)
	log.Printf("Target Msgs/Sec: %v\n", c.targetPubRate)
	log.Printf("Target Band/Sec: %v\n", c.byteSize(c.targetPubRate*c.msgSize*2))
	log.Println("==============================")

	// Random payload.
	data := make([]byte, c.msgSize)
	io.ReadFull(rand.Reader, data)

	// For publish throttling.
	delay := time.Second / time.Duration(c.targetPubRate)
	pubStart := time.Now()

	// Throttle logic.
	minDelay := time.Duration(c.minDelay)
	adjustAndSleep := func(count int) {
		r := c.rps(count, time.Since(pubStart))
		adj := delay / 20 // 5%
		if adj == 0 {
			adj = minDelay
		}
		if r < c.targetPubRate {
			delay -= adj
		} else if r > c.targetPubRate {
			delay += adj
		}
		if delay < 0 {
			delay = 0
		}
		time.Sleep(delay)
	}

	// Now publish.
	var lastDelay time.Time
	var totalStalls int
	var stallTime time.Duration
	stall := time.Duration(c.stall) * time.Millisecond
	for i := 0; i < c.numPubs; i++ {
		if maxInflight > 0 {
			if cin := atomic.LoadInt64(&inflight); cin > 0 && cin > maxInflight/4 && time.Since(lastDelay) > stall {
				mu.Lock()
				// fmt.Printf("*")
				totalStalls++
				t0 := time.Now()
				cond.Wait()
				stallTime += time.Since(t0)
				mu.Unlock()
				lastDelay = time.Now()
			}
		}

		now := time.Now()
		// Place the send time in the front of the reply subject.
		err = c1.PublishRequest(subject, fmt.Sprintf("%s.%d", inbox, now.UnixNano()), data)
		if err != nil {
			log.Printf("Publishing failed: %v", err)
		}
		if maxInflight > 0 {
			atomic.AddInt64(&inflight, 1)
		}
		adjustAndSleep(i + 1)
	}
	pubDur := time.Since(pubStart)
	wg.Wait()
	ackDur := time.Since(pubStart)
	subDur := time.Since(firstAckTime)

	// If we are writing to files, save the original unsorted data
	if c.histFile != "" {
		if err := c.writeRawFile(c.histFile+".raw", durations); err != nil {
			log.Printf("Unable to write raw output file: %v", err)
		}
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	h := hdrhistogram.New(1, int64(durations[len(durations)-1]), 5)
	for _, d := range durations {
		h.RecordValue(int64(d))
	}

	fmt.Println()
	log.Printf("HDR Percentiles:\n")
	log.Printf("1:        %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(1))))
	log.Printf("2:        %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(2))))
	log.Printf("5:        %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(5))))
	log.Printf("10:       %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(10))))
	log.Printf("25:       %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(25))))
	log.Printf("50:       %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(50))))
	log.Printf("75:       %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(75))))
	log.Printf("90:       %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(90))))
	log.Printf("95:       %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(95))))
	log.Printf("99:       %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(99))))
	log.Printf("99.9:     %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(99.9))))
	log.Printf("99.99:    %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(99.99))))
	log.Printf("99.999:   %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(99.999))))
	log.Printf("99.9999:  %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(99.9999))))
	log.Printf("99.99999: %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(99.99999))))
	log.Printf("100:      %v\n", c.fmtDur(time.Duration(h.ValueAtQuantile(100.0))))
	log.Println("==============================")

	if c.histFile != "" {
		pctls := histwriter.Percentiles{10, 25, 50, 75, 90, 99, 99.9, 99.99, 99.999, 99.9999, 99.99999, 100.0}
		histwriter.WriteDistributionFile(h, pctls, 1.0/1000000.0, c.histFile+".histogram")
	}

	// Print results
	med, err := c.getMedian(durations)
	if err != nil {
		return err
	}
	log.Printf("Pub Msgs/Sec        : %d\n", c.rps(c.numPubs, pubDur))
	log.Printf("Pub Band/Sec        : %v\n", c.byteSize(c.rps(c.numPubs, pubDur)*c.msgSize*2))
	log.Printf("Ack Msgs/Sec        : %d\n", c.rps(c.numPubs, subDur))
	log.Printf("Ack Band/Sec        : %v\n", c.byteSize(c.rps(c.numPubs, subDur)*c.msgSize*2))
	log.Printf("Minimum Ack Latency : %v", c.fmtDur(durations[0]))
	log.Printf("Median Ack Latency  : %v", c.fmtDur(med))
	log.Printf("Maximum Ack Latency : %v", c.fmtDur(durations[len(durations)-1]))
	log.Printf("1st Sent Wall Time  : %v", c.fmtDur(pubStart.Sub(start)))
	log.Printf("Last Sent Wall Time : %v", c.fmtDur(pubDur))
	log.Printf("Last Recv Wall Time : %v", c.fmtDur(subDur))
	log.Printf("Acks Wait Time      : %v", c.fmtDur(ackDur-pubDur))
	log.Printf("Stalls              : %v", totalStalls)
	log.Printf("Stalls/Sec          : %v", c.rps(totalStalls, pubDur))
	log.Printf("Stalls Time         : %v", stallTime)

	return nil
}

// Just pretty print the byte sizes.
func (c *perfCmd) byteSize(n int) string {
	sizes := []string{"B", "K", "M", "G", "T"}
	base := float64(1024)
	if n < 10 {
		return fmt.Sprintf("%d%s", n, sizes[0])
	}
	e := math.Floor(c.logn(float64(n), base))
	suffix := sizes[int(e)]
	val := math.Floor(float64(n)/math.Pow(base, e)*10+0.5) / 10
	f := "%.0f%s"
	if val < 10 {
		f = "%.1f%s"
	}
	return fmt.Sprintf(f, val, suffix)
}

func (c *perfCmd) logn(n, b float64) float64 {
	return math.Log(n) / math.Log(b)
}

func (c *perfCmd) getMedian(values []time.Duration) (time.Duration, error) {
	l := len(values)
	if l == 0 {
		return 0, fmt.Errorf("empty set")
	}
	if l%2 == 0 {
		return (values[l/2-1] + values[l/2]) / 2, nil
	}
	return values[l/2], nil
}

// waitForRoute tests a subscription in the server to ensure subject interest
// has been propagated between servers.  Otherwise, we may miss early messages
// when testing with clustered servers and the test will hang.
func (c *perfCmd) waitForRoute(pnc *nats.Conn, snc *nats.Conn) error {
	// No need to continue if using one server
	if strings.Compare(pnc.ConnectedServerId(), snc.ConnectedServerId()) == 0 {
		return nil
	}

	// Setup a test subscription to let us know when a message has been received.
	// Use a new inbox subject as to not skew results
	var routed int32
	subject := snc.NewRespInbox()
	sub, err := snc.Subscribe(subject, func(msg *nats.Msg) {
		atomic.AddInt32(&routed, 1)
	})
	if err != nil {
		return fmt.Errorf("couldn't subscribe to test subject %s: %v", subject, err)
	}
	defer sub.Unsubscribe()
	err = snc.Flush()
	if err != nil {
		return fmt.Errorf("flushing connection failed: %v", err)
	}

	// Periodically send messages until the test subscription receives
	// a message.  Allow for two seconds.
	start := time.Now()
	for atomic.LoadInt32(&routed) == 0 {
		if time.Since(start) > (time.Second * 2) {
			return fmt.Errorf("couldn't receive end-to-end test message")
		}
		if err = pnc.Publish(subject, nil); err != nil {
			return fmt.Errorf("couldn't publish to test subject %s:  %v", subject, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

// Make time durations a bit prettier.
func (c *perfCmd) fmtDur(t time.Duration) time.Duration {
	// e.g 234us, 4.567ms, 1.234567s
	return t.Truncate(time.Microsecond)
}

func (c *perfCmd) rps(count int, elapsed time.Duration) int {
	return int(float64(count) / (float64(elapsed) / float64(time.Second)))
}

// writeRawFile creates a file with a list of recorded latency
// measurements, one per line.
func (c *perfCmd) writeRawFile(filePath string, values []time.Duration) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, value := range values {
		fmt.Fprintf(f, "%f\n", float64(value.Nanoseconds())/1000000.0)
	}
	return nil
}
