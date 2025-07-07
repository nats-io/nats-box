package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
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
		subject       = flag.String("subject", "", "Subject to publish to")
		serverURL     = flag.String("server", "nats://localhost:4222", "NATS server URL")
		numClients    = flag.Int("clients", 1, "Number of concurrent clients")
		numMsg        = flag.Int("msgs", 100000, "Number of messages to publish")
		msgSize       = flag.Int("size", 128, "Size of the test messages")
		progressBar   = flag.Bool("progress", true, "Enable progress bar")
		sleepDuration = flag.Duration("sleep", 0, "Sleep between publications")
		payloadFile   = flag.String("payload", "", "File containing message payload")
	)
	flag.Parse()

	if *subject == "" {
		log.Fatal("Subject is required")
	}

	if *numMsg <= 0 {
		log.Fatal("Number of messages should be greater than 0")
	}

	if *msgSize <= 0 || *msgSize > math.MaxInt {
		log.Fatal("Invalid message size")
	}

	log.Printf("Starting Core NATS publish benchmark [clients=%d, msg-size=%s, msgs=%s, sleep=%v, subject=%s]",
		*numClients, humanize.Bytes(uint64(*msgSize)), humanize.Comma(int64(*numMsg)), *sleepDuration, *subject)

	bm := bench.NewBenchmark("NATS", 0, *numClients)

	startwg := &sync.WaitGroup{}
	donewg := &sync.WaitGroup{}

	pubCounts := bench.MsgsPerClient(*numMsg, *numClients)
	trigger := make(chan struct{})
	errChan := make(chan error, *numClients)

	for i := 0; i < *numClients; i++ {
		nc, err := nats.Connect(*serverURL)
		if err != nil {
			log.Fatal(err)
		}
		defer nc.Close()

		startwg.Add(1)
		donewg.Add(1)

		go runCorePublisher(bm, errChan, nc, startwg, donewg, trigger, pubCounts[i], offset(i, pubCounts), strconv.Itoa(i), *subject, *msgSize, *sleepDuration, *payloadFile, *progressBar)
	}

	if *progressBar {
		uiprogress.Start()
	}

	startwg.Wait()
	close(trigger)
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

func offset(putter int, counts []int) int {
	var position = 0
	for i := 0; i < putter; i++ {
		position = position + counts[i]
	}
	return position
}

func runCorePublisher(bm *bench.Benchmark, errChan chan error, nc *nats.Conn, startwg *sync.WaitGroup, donewg *sync.WaitGroup, trigger chan struct{}, numMsg int, offset int, pubNumber string, subject string, msgSize int, sleepDuration time.Duration, payloadFile string, showProgress bool) {
	startwg.Done()

	var progress *uiprogress.Bar

	log.Printf("Starting publisher, publishing %s messages", humanize.Comma(int64(numMsg)))

	if showProgress {
		progress = uiprogress.AddBar(numMsg).AppendCompleted().PrependElapsed()
		progress.Width = progressWidth()
	}

	<-trigger

	start := time.Now()
	err := coreNATSPublisher(nc, progress, msgSize, numMsg, offset, subject, sleepDuration, payloadFile)
	if err != nil {
		errChan <- fmt.Errorf("publishing: %w", err)
		donewg.Done()
		return
	}

	err = nc.Flush()
	if err != nil {
		errChan <- fmt.Errorf("flushing: %w", err)
		donewg.Done()
		return
	}

	bm.AddPubSample(bench.NewSample(numMsg, msgSize, start, time.Now(), nc))

	donewg.Done()
	errChan <- nil
}

func coreNATSPublisher(nc *nats.Conn, progress *uiprogress.Bar, payloadSize int, numMsg int, offset int, subject string, sleepDuration time.Duration, payloadFile string) error {
	state := "Publishing"

	var payload []byte
	var err error

	if payloadFile != "" {
		payload, err = os.ReadFile(payloadFile)
		if err != nil {
			return fmt.Errorf("reading payload file: %w", err)
		}
	} else {
		payload = make([]byte, payloadSize)
		for i := 0; i < payloadSize; i++ {
			payload[i] = 'A' + byte(i%26)
		}
	}

	message := nats.Msg{Data: payload, Subject: subject}

	if progress != nil {
		progress.PrependFunc(func(b *uiprogress.Bar) string {
			return state
		})
	}

	for i := 0; i < numMsg; i++ {
		if progress != nil {
			progress.Incr()
		}

		err := nc.PublishMsg(&message)
		if err != nil {
			return fmt.Errorf("publishing: %w", err)
		}

		time.Sleep(sleepDuration)
	}

	state = "Finished  "
	return nil
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