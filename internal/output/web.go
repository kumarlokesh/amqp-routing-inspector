package output

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
	"github.com/kumarlokesh/amqp-routing-inspector/internal/web"
)

// webRenderer serves a live routing graph over HTTP using Server-Sent Events (SSE).
// Connect a browser to the configured address to view the graph.
type webRenderer struct {
	writer io.Writer // unused when server runs; retained for interface compat
	addr   string
	server *http.Server

	mu      sync.Mutex
	closed  bool
	clients map[chan string]struct{}
}

func newWebRenderer(writer io.Writer, addr string) (*webRenderer, error) {
	r := &webRenderer{
		writer:  writer,
		addr:    addr,
		clients: make(map[chan string]struct{}),
	}

	if addr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/", r.serveIndex)
		mux.HandleFunc("/events", r.serveSSE)
		r.server = &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		ln, err := listenTCP(addr)
		if err != nil {
			return nil, fmt.Errorf("start web server on %s: %w", addr, err)
		}
		go func() { _ = r.server.Serve(ln) }()
	}

	return r, nil
}

func (r *webRenderer) serveIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(web.Page)
}

func (r *webRenderer) serveSSE(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan string, 32)

	r.mu.Lock()
	r.clients[ch] = struct{}{}
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.clients, ch)
		r.mu.Unlock()
	}()

	flusher, hasFlusher := w.(http.Flusher)
	for {
		select {
		case <-req.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			if hasFlusher {
				flusher.Flush()
			}
		}
	}
}

func (r *webRenderer) RenderTrace(trace model.RoutingTrace) error {
	payload, err := json.Marshal(trace)
	if err != nil {
		return fmt.Errorf("marshal trace for SSE: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}

	msg := string(payload)
	for ch := range r.clients {
		select {
		case ch <- msg:
		default:
			// Slow client: drop the event rather than block.
		}
	}
	return nil
}

func (r *webRenderer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true

	// Close all SSE client channels so their goroutines exit.
	for ch := range r.clients {
		close(ch)
		delete(r.clients, ch)
	}

	if r.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.server.Shutdown(ctx)
	}

	return nil
}
