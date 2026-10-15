package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"internship-tracker/internal/crawler"
	"internship-tracker/internal/model"
	"internship-tracker/internal/store"
)

const dbPath = "jobs.db"

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.Println("crawler: starting")

	// Open (or create) the shared SQLite database.
	s, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("crawler: open store: %v", err)
	}
	defer s.Close()

	// Respect SIGINT/SIGTERM — cancel all crawlers cleanly on shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// jobs is the fan-in channel from all three ATS crawlers.
	// Buffered so writers don't block while the DB writer catches up.
	jobs := make(chan model.Job, 128)

	// writerDone is closed once the DB writer goroutine has finished
	// flushing everything, so we don't exit before the last row is written.
	writerDone := make(chan struct{})

	// --- DB writer goroutine ---
	// Drains the jobs channel and writes each job to SQLite.
	// Runs as a single goroutine — SQLite writes are serialised here,
	// which is both correct and performant for our write volume.
	go func() {
		defer close(writerDone)
		written, skipped := 0, 0
		for job := range jobs {
			if err := s.Insert(job); err != nil {
				log.Printf("crawler: insert %s: %v", job.ID, err)
				skipped++
				continue
			}
			written++
			log.Printf("crawler: saved [%s] %s @ %s (%s)", job.Source, job.Title, job.Company, job.LocationRaw)
		}
		log.Printf("crawler: writer done — %d written, %d skipped", written, skipped)
	}()

	// --- Fan-out crawlers ---
	// crawler.Run fans out goroutines for Greenhouse, Lever, and Workday,
	// deduplicates results via sync.Map, and closes jobs when all are done.
	if err := crawler.Run(ctx, jobs); err != nil {
		log.Printf("crawler: run error: %v", err)
	}

	// Signal the DB writer that no more jobs are coming, then wait for it.
	close(jobs)
	<-writerDone

	count, _ := s.Count()
	log.Printf("crawler: done — %d total jobs in database", count)
}
