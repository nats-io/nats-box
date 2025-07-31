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
	"crypto/tls"
	"fmt"
	iu "github.com/nats-io/nats-box/internal/util"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/choria-io/fisk"
	"github.com/nats-io/nats.go"
)

type rttCmd struct {
	iterations int
	json       bool
}

type rttResult struct {
	Time    time.Time     `json:"time"`
	Address string        `json:"address"`
	RTT     time.Duration `json:"rtt"`
	URL     string        `json:"url"`
}

type rttTarget struct {
	URL     string       `json:"url"`
	Results []*rttResult `json:"results"`
	tlsName string
}

func configureRTTCommand(app commandHost) {
	c := &rttCmd{}

	rtt := app.Command("rtt", "Compute round-trip time to NATS server").Action(c.rtt)
	rtt.Arg("iterations", "How many round trips to do when testing").Default("5").IntVar(&c.iterations)
	rtt.Flag("json", "Produce JSON output").Short('j').UnNegatableBoolVar(&c.json)
}

func init() {
	registerCommand("rtt", 13, configureRTTCommand)
}

func (c *rttCmd) rtt(_ *fisk.ParseContext) error {
	targets, err := c.targets()
	if err != nil {
		return err
	}

	err = c.performTest(targets)
	if err != nil {
		return err
	}

	if c.json {
		iu.PrintJSON(targets)

		return nil
	}

	f := fmt.Sprintf("%%%ds: %%v\n", c.calcIndent(targets, 3))

	for _, t := range targets {
		fmt.Printf("%s:\n\n", t.URL)

		for _, r := range t.Results {
			fmt.Printf(f, r.Address, r.RTT)
		}

		fmt.Println()
	}

	return nil
}

func (c *rttCmd) calcIndent(targets []*rttTarget, prefix int) int {
	i := prefix

	for _, t := range targets {
		for _, r := range t.Results {
			p := len(r.Address) + prefix

			if p > i {
				i = p
			}
		}
	}

	return i
}

func (c *rttCmd) performTest(targets []*rttTarget) (err error) {
	for _, target := range targets {
		opts := natsOpts()
		if target.tlsName != "" {
			opts = append(opts, func(o *nats.Options) error {
				if o.TLSConfig == nil {
					o.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: target.tlsName}
				} else {
					o.TLSConfig.ServerName = target.tlsName
				}

				return nil
			})
		}

		for _, r := range target.Results {
			r.Time = time.Now()
			r.URL, r.RTT, err = c.calcRTT(r.Address, opts)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *rttCmd) calcRTT(server string, copts []nats.Option) (string, time.Duration, error) {
	opts().Conn = nil

	if opts().Trace {
		log.Printf(">>> Connecting to %s\n", server)
	}

	nc, err := newNatsConn(server, copts...)
	if err != nil {
		return "", 0, err
	}
	defer nc.Close()

	time.Sleep(25 * time.Millisecond)

	var totalTime time.Duration

	if opts().Trace {
		fmt.Printf("RTT iterations for server: %s\n", server)
	}
	for i := 1; i <= c.iterations; i++ {
		rtt, err := nc.RTT()
		if err != nil {
			return "", 0, fmt.Errorf("rtt failed: %v", err)
		}

		totalTime += rtt
		if opts().Trace {
			fmt.Printf("#%d:\trtt=%s\n", i, rtt)
			if i == c.iterations {
				fmt.Println()
			}
		}
	}

	return nc.ConnectedUrl(), totalTime / time.Duration(c.iterations), nil
}

func (c *rttCmd) targets() (targets []*rttTarget, err error) {
	servers := ""
	if opts().Conn != nil {
		servers = strings.Join(opts().Conn.DiscoveredServers(), ",")
	} else if opts().Config != nil {
		servers = opts().Config.ServerURL()
	} else {
		return nil, fmt.Errorf("cannot find a server list to test")
	}

	for _, s := range strings.Split(servers, ",") {
		if !strings.Contains(s, "://") {
			s = fmt.Sprintf("nats://%s", s)
		}

		u, err := url.Parse(s)
		if err != nil {
			return targets, err
		}

		port := u.Port()
		if port == "" {
			port = "4222"
		}

		targets = append(targets, &rttTarget{URL: u.String()})
		target := targets[len(targets)-1]

		// its a ip just add it
		if net.ParseIP(u.Hostname()) != nil {
			target.Results = append(target.Results, &rttResult{Address: fmt.Sprintf("%s://%s", u.Scheme, net.JoinHostPort(u.Hostname(), port))})
			continue
		}

		// else look it up and add all its addresses
		addrs, _ := net.LookupHost(u.Hostname())
		if len(addrs) == 0 {
			target.Results = append(target.Results, &rttResult{Address: fmt.Sprintf("%s://%s", u.Scheme, net.JoinHostPort(u.Hostname(), port))})
			continue
		}

		// if we have many addresses we'll connect to each IP but we have to use the
		// name of the original server address to do validate TLS, connect here, check it
		// requires TLS and store the name to use when connecting to each IP
		target.tlsName = u.Hostname()

		for _, a := range addrs {
			target.Results = append(target.Results, &rttResult{Address: fmt.Sprintf("%s://%s", u.Scheme, net.JoinHostPort(a, port))})
		}
	}

	return targets, nil
}
