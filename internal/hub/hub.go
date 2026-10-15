package hub

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"internship-tracker/internal/model"
)

// client represents a single connected WebSocket client.
// Each client has its own send channel so a slow reader
// doesn't block broadcasts to other clients.
type client struct {
	send chan []byte
	conn WSConn
}

// WSConn is the minimal WebSocket interface the hub needs.
// Using an interface makes the hub testable without a real HTTP connection.
type WSConn interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

const (
	sendBufSize  = 64  // per-client outbound buffer (messages)
	wsTextMessage = 1  // websocket.TextMessage constant value
)

// Hub manages the set of active WebSocket clients and broadcasts
// job messages to all of them.
//
// Concurrency model:
//   - register/unregister/broadcast all go through a single select loop
//     in Run(), so the clients map is never touched from multiple goroutines.
//   - Each client's send channel is written by the hub loop and drained
//     by that client's dedicated write goroutine — no shared state.
type Hub struct {
	clients    map[*client]struct{}
	register   chan *client
	unregister chan *client
	broadcast  chan []byte
	mu         sync.RWMutex // guards len(clients) reads from outside Run()
}

// New creates a Hub ready to be started with Run().
func New() *Hub {
	return &Hub{
		clients:    make(map[*client]struct{}),
		register:   make(chan *client, 8),
		unregister: make(chan *client, 8),
		broadcast:  make(chan []byte, 256),
	}
}

// Run starts the hub's event loop. It blocks until ctx is done —
// call it in its own goroutine.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = struct{}{}
			h.mu.Unlock()
			log.Printf("hub: client connected (%d total)", len(h.clients))

		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
			log.Printf("hub: client disconnected (%d total)", len(h.clients))

		case msg := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// Client's buffer is full — it's too slow.
					// Drop this message for that client rather than
					// blocking the entire broadcast loop.
					log.Printf("hub: dropped message for slow client")
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast serialises a Job to JSON and enqueues it for all connected clients.
func (h *Hub) Broadcast(job model.Job) {
	data, err := json.Marshal(job)
	if err != nil {
		log.Printf("hub: marshal error: %v", err)
		return
	}
	h.broadcast <- data
}

// ClientCount returns the number of currently connected WebSocket clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}


// FlushTo writes all jobs directly to a single connection before it is
// registered with the hub. Used to send the existing DB backlog to a
// newly connected client without going through the broadcast channel.
func (h *Hub) FlushTo(conn WSConn, jobs []model.Job) {
	for _, job := range jobs {
		data, err := json.Marshal(job)
		if err != nil {
			log.Printf("hub: flush marshal error: %v", err)
			continue
		}
		if err := conn.WriteMessage(wsTextMessage, data); err != nil {
			log.Printf("hub: flush write error: %v", err)
			return
		}
	}
}

// ServeWS upgrades an HTTP request to a WebSocket connection and registers
// the resulting client with the hub.
//
// Caller is responsible for providing an Upgrader — this keeps the hub
// decoupled from any specific WebSocket library.
func (h *Hub) ServeWS(conn WSConn) {
	c := &client{
		send: make(chan []byte, sendBufSize),
		conn: conn,
	}

	h.register <- c

	// writePump drains c.send and writes each message to the WebSocket.
	// Runs in its own goroutine so it never blocks the hub loop.
	go func() {
		defer func() {
			h.unregister <- c
			conn.Close()
		}()
		for msg := range c.send {
			if err := conn.WriteMessage(wsTextMessage, msg); err != nil {
				log.Printf("hub: write error: %v", err)
				return
			}
		}
	}()

	// readPump keeps the connection alive and detects client disconnects.
	// WebSocket requires the server to read even if it only writes,
	// otherwise the underlying TCP buffer fills and stalls.
	go func() {
		defer func() {
			h.unregister <- c
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				// EOF or close frame — client disconnected.
				return
			}
		}
	}()
}

// ServeHTTP is a convenience wrapper that upgrades the request using
// the standard gorilla/nhooyr-compatible upgrade func passed by the caller.
func ServeHTTP(
	h *Hub,
	upgrader func(w http.ResponseWriter, r *http.Request) (WSConn, error),
	w http.ResponseWriter,
	r *http.Request,
) {
	conn, err := upgrader(w, r)
	if err != nil {
		log.Printf("hub: upgrade error: %v", err)
		return
	}
	h.ServeWS(conn)
}
