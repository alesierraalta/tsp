// Command relayd runs the import pipeline and drains the delivery queue.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"example.com/relay"
	"example.com/relay/internal/config"
	"example.com/relay/internal/jobs"
)

func main() {
	var (
		interval = flag.String("interval", "30s", "how often the queue is drained")
		imports  = flag.String("import", "", "a tar stream to import before serving")
	)
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "relayd:", err)
		os.Exit(2)
	}
	schedule, err := jobs.ParseSchedule(*interval)
	if err != nil {
		fmt.Fprintln(os.Stderr, "relayd:", err)
		os.Exit(2)
	}
	service, err := relay.Open(relay.Options{
		Staging:   cfg.Staging,
		Allowlist: cfg.Allowlist,
		Attempts:  cfg.Attempts,
		Backoff:   cfg.Backoff,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "relayd:", err)
		os.Exit(2)
	}

	if *imports != "" {
		file, err := os.Open(*imports)
		if err != nil {
			fmt.Fprintln(os.Stderr, "relayd:", err)
			os.Exit(1)
		}
		report, err := service.ImportStream(io.Reader(file))
		file.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "relayd: import:", err)
			os.Exit(1)
		}
		log.Printf("imported %d entries, %d records into %s", report.Written, report.Records, service.Staging())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ticker := newTicker(schedule, func() {
		results, err := service.RunQueue(ctx)
		if err != nil {
			log.Printf("queue: %v", err)
		}
		log.Printf("queue: %d jobs, %d owed", len(results), service.Pending())
	})
	<-ctx.Done()
	ticker.Stop()
	log.Printf("relayd: stopping with %d jobs owed", service.Pending())
}

type ticker struct {
	done chan struct{}
}

func newTicker(schedule jobs.Schedule, run func()) *ticker {
	t := &ticker{done: make(chan struct{})}
	go func() {
		for {
			select {
			case <-t.done:
				return
			case <-timeAfter(schedule.Every):
				run()
			}
		}
	}()
	return t
}

func (t *ticker) Stop() { close(t.done) }
