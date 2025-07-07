package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gosuri/uiprogress"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/bench"
	terminal "golang.org/x/term"
)

func main() {
	var (
		subject     = flag.String("subject", "", "Subject to subscribe to")
		serverURL   = flag.String("server", "nats://localhost:4222", "NATS server URL")
		numClients  = flag.Int("clients", 1, "Number of concurrent clients")
		numMsg      = flag.Int("msgs", 100000, "Number of messages to receive")
		progressBar = flag.Bool("progress", true, "Enable progress bar")
	)
	flag.Parse()

	if *subject == "" {
		log.Fatal("Subject is required")
	}

	if *numMsg <= 0 {
		log.Fatal("Number of messages should be greater than 0")
	}

	log.Printf("Starting Core NATS subscribe benchmark [clients=%d, msgs=%s, subject=%s]",
		*numClients, humanize.Comma(int64(*numMsg)), *subject)

	bm := bench.NewBenchmark("NATS", *numClients, 0)

	startwg := &sync.WaitGroup{}
	donewg := &sync.WaitGroup{}
	errChan := make(chan error, *numClients)

	for i := 0; i < *numClients; i++ {
		nc, err := nats.Connect(*serverURL)
		if err != nil {
			log.Fatalf("Client number %d failed to connect: %v", i, err)
		}
		defer nc.Close()

		startwg.Add(1)
		donewg.Add(1)

		go runCoreSubscriber(bm, errChan, nc, startwg, donewg, *numMsg, *subject, *progressBar)
	}

	if *progressBar {
		uiprogress.Start()
	}

	startwg.Wait()
	donewg.Wait()

	var err2 error
	for i := 0; i < *numClients; i++ {
		if err := <-errChan; err != nil {
			log.Printf("Error from client %d: %v", i, err)
			if err2 == nil {
				err2 = err
			}
		}
	}

	if err2 != nil {
		log.Fatal(err2)
	}

	bm.Close()
	printResults(bm)
}

func runCoreSubscriber(bm *bench.Benchmark, errChan chan error, nc *nats.Conn, startwg *sync.WaitGroup, donewg *sync.WaitGroup, numMsg int, subject string, showProgress bool) {
	received := 0
	ch := make(chan time.Time, 2)
	var progress *uiprogress.Bar

	if showProgress {
		progress = uiprogress.AddBar(numMsg).AppendCompleted().PrependElapsed()
		progress.Width = progressWidth()
	}

	state := "Setup     "

	if progress != nil {
		progress.PrependFunc(func(b *uiprogress.Bar) string {
			return state
		})
	}

	// Core NATS Message handler
	mh := func(msg *nats.Msg) {
		received++

		if received == 1 {
			ch <- time.Now()
		}

		if received >= numMsg {
			ch <- time.Now()
		}

		if progress != nil {
			progress.Incr()
		}
	}

	state = "Receiving "

	sub, err := nc.Subscribe(subject, mh)
	if err != nil {
		errChan <- fmt.Errorf("subscribing to '%s': %w", subject, err)
		startwg.Done()
		donewg.Done()
		return
	}

	err = sub.SetPendingLimits(-1, -1)
	if err != nil {
		errChan <- fmt.Errorf("setting pending limits on the subscriber: %w", err)
		startwg.Done()
		donewg.Done()
		return
	}

	err = nc.Flush()
	if err != nil {
		errChan <- fmt.Errorf("flushing: %w", err)
		startwg.Done()
		donewg.Done()
		return
	}

	startwg.Done()

	start := <-ch
	end := <-ch

	state = "Finished  "

	bm.AddSubSample(bench.NewSample(numMsg, 0, start, end, nc))

	donewg.Done()
	errChan <- nil
}

func progressWidth() int {
	w, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80
	}

	minWidth := 10

	if w-30 <= minWidth {
		return minWidth
	} else {
		return w - 30
	}
}

func printResults(bm *bench.Benchmark) {
	fmt.Println()
	fmt.Println(bm.Report())
}