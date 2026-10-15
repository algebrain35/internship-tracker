package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"internship-tracker/internal/hub"
	"internship-tracker/internal/store"
)

const (
	dbPath       = "jobs.db"
	addr         = ":8080"
	pollInterval = 2 * time.Second
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.Println("server: starting on", addr)

	s, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("server: open store: %v", err)
	}
	defer s.Close()

	h := hub.New()
	go h.Run()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- SQLite poller ---
	// Broadcasts only *new* rows (rowid > lastRowID) to all clients.
	// lastRowID advances each tick, so rows are never double-sent.
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		var lastRowID int64
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				jobs, maxRowID, err := s.PollSince(lastRowID)
				if err != nil {
					log.Printf("server: poll error: %v", err)
					continue
				}
				if len(jobs) == 0 {
					continue
				}
				log.Printf("server: broadcasting %d new job(s) to %d client(s)", len(jobs), h.ClientCount())
				for _, job := range jobs {
					h.Broadcast(job)
				}
				lastRowID = maxRowID
			}
		}
	}()

	// --- HTTP routes ---
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("web")))

	// FIX: on WebSocket connect, flush all existing jobs to that client
	// before handing it to the hub. This ensures clients that connect
	// after the crawler finishes still receive the full backlog.
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrade(w, r)
		if err != nil {
			log.Printf("server: upgrade error: %v", err)
			return
		}

		// Send all existing jobs to this client before registering with hub.
		existing, _, err := s.PollSince(0)
		if err != nil {
			log.Printf("server: backlog fetch error: %v", err)
		} else {
			log.Printf("server: sending %d backlog job(s) to new client", len(existing))
			h.FlushTo(conn, existing)
		}

		// Now register with hub for future broadcasts.
		h.ServeWS(conn)
	})

	srv := &http.Server{
		Addr:        addr,
		Handler:     mux,
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 0,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	go func() {
		<-ctx.Done()
		log.Println("server: shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Printf("server: shutdown error: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: listen: %v", err)
	}
	log.Println("server: stopped")
}

