// cmd/server/sse.go
//
// SSEHub manages Server-Sent Events (SSE) connections.
// It is the single broadcast point for real-time pattern and stats events.
//
// Design:
//   - Each connected client gets a buffered channel (cap 64).
//     The buffer absorbs bursts; a slow client is skipped, not allowed to block.
//   - Register/Unregister are called from the HTTP handler goroutine.
//   - Broadcast is called from the consumer goroutine (different goroutine) —
//     hence the mutex.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"behaviourlens/internal/models"
)

// SSEHub is a thread-safe broadcast hub for SSE clients.
type SSEHub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

// newSSEHub constructs an initialised hub.
func newSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan string]struct{}),
	}
}

// Register adds a client channel to the broadcast set.
func (h *SSEHub) Register(ch chan string) {
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
}

// Unregister removes the channel and closes it so the HTTP handler can exit.
func (h *SSEHub) Unregister(ch chan string) {
	h.mu.Lock()
	delete(h.clients, ch)
	close(ch)
	h.mu.Unlock()
}

// Broadcast sends msg to every registered client.
// If a client's buffer is full the message is dropped for that client only.
func (h *SSEHub) Broadcast(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
			// Slow client — drop rather than block.
		}
	}
}

// BroadcastPattern serialises a Pattern and broadcasts it as a "pattern" SSE event.
func (h *SSEHub) BroadcastPattern(p models.Pattern) {
	b, err := json.Marshal(p)
	if err != nil {
		log.Printf("sse: marshal pattern error: %v", err)
		return
	}
	// SSE format: "event: <name>\ndata: <json>\n\n"
	h.Broadcast(fmt.Sprintf("event: pattern\ndata: %s\n\n", b))
}

// BroadcastStats serialises a SystemMetrics snapshot and broadcasts it as a "stats" SSE event.
func (h *SSEHub) BroadcastStats(m models.SystemMetrics) {
	b, err := json.Marshal(m)
	if err != nil {
		log.Printf("sse: marshal stats error: %v", err)
		return
	}
	h.Broadcast(fmt.Sprintf("event: stats\ndata: %s\n\n", b))
}

// StartStatsTicker launches a goroutine that broadcasts SystemMetrics every interval.
// It uses the provided metrics function rather than importing state directly.
func (h *SSEHub) StartStatsTicker(interval time.Duration, getMetrics func() models.SystemMetrics) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			h.BroadcastStats(getMetrics())
		}
	}()
}

// ── HTTP handler ──────────────────────────────────────────────────────────────

// streamHandler implements GET /api/stream.
// It upgrades the connection to SSE and streams events until the client disconnects.
func streamHandler(hub *SSEHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Confirm the client supports SSE (optional but good practice).
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Set SSE headers.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		// Already set by CORS middleware, but belt-and-suspenders.
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Allocate a client channel and register it.
		ch := make(chan string, 64)
		hub.Register(ch)
		defer hub.Unregister(ch)

		log.Printf("sse: client connected (%s)", r.RemoteAddr)

		// Send an initial "connected" comment so the browser knows the stream is live.
		fmt.Fprintf(w, ": connected\n\n")
		flusher.Flush()

		// Stream messages until the client disconnects.
		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				log.Printf("sse: client disconnected (%s)", r.RemoteAddr)
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprint(w, msg)
				flusher.Flush()
			}
		}
	}
}
