package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"internship-tracker/internal/crawler"
	"internship-tracker/internal/model"
	"internship-tracker/internal/store"
)

const dbPath = "jobs.db"

func main() {
	every := flag.Duration("every", 0, "re-crawl interval, e.g. 15m (0 = crawl once and exit)")
	flag.Parse()

	log.SetFlags(log.Ltime | log.Lshortfile)
	log.Println("crawler: starting")

	s, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("crawler: open store: %v", err)
	}
	defer s.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for {
		crawlOnce(ctx, s)
		if *every <= 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(*every):
		}
	}
}

// crawlOnce creates its own jobs channel and closes it itself, only after
// crawler.Run has returned. Run returns once every sender has finished, so
// closing earlier (or reusing the channel for a second crawl) panics with
// "send on closed channel".
func crawlOnce(ctx context.Context, s *store.Store) {
	runStart := time.Now().UTC()

	jobs := make(chan model.Job, 128)
	writerDone := make(chan struct{})

	var written, skipped int
	seen := map[string]bool{} // sources that returned at least one job this run

	go func() {
		defer close(writerDone)
		for job := range jobs {
			if err := s.Insert(job); err != nil {
				log.Printf("crawler: insert %s: %v", job.ID, err)
				skipped++
				continue
			}
			written++
			seen[string(job.Source)] = true
		}
		log.Printf("crawler: writer done: %d written, %d skipped", written, skipped)
	}()

	runErr := crawler.Run(ctx, jobs) // blocks until all senders are done
	close(jobs)                      // safe now: nobody can send anymore
	<-writerDone                     // wait for the last rows to be written

	switch {
	case runErr != nil:
		log.Printf("crawler: run failed, skipping close pass: %v", runErr)
	case ctx.Err() != nil:
		log.Println("crawler: interrupted, skipping close pass")
	default:
		srcs := make([]string, 0, len(seen))
		for src := range seen {
			srcs = append(srcs, src)
		}
		n, err := s.MarkClosed(runStart, srcs)
		log.Printf("crawler: marked %d closed (err=%v)", n, err)
	}

	count, _ := s.Count()
	log.Printf("crawler: done, %d total jobs in database", count)
}