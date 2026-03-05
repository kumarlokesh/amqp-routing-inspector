package output

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
)

type exchangeEventKey struct {
	exchange  string
	eventType string
}

type exchangeQueueKey struct {
	exchange string
	queue    string
}

type prometheusRenderer struct {
	writer io.Writer
	addr   string
	server *http.Server

	mu                    sync.Mutex
	closed                bool
	eventsByExchangeType  map[exchangeEventKey]int64
	deliveriesByExchangeQ map[exchangeQueueKey]int64
	unroutedByExchange    map[string]int64
}

func newPrometheusRenderer(writer io.Writer, addr string) (*prometheusRenderer, error) {
	r := &prometheusRenderer{
		writer:                writer,
		addr:                  addr,
		eventsByExchangeType:  make(map[exchangeEventKey]int64),
		deliveriesByExchangeQ: make(map[exchangeQueueKey]int64),
		unroutedByExchange:    make(map[string]int64),
	}

	if addr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/metrics", r.serveMetrics)
		r.server = &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		if err := r.startServer(); err != nil {
			return nil, fmt.Errorf("start prometheus server on %s: %w", addr, err)
		}
	}

	return r, nil
}

func (r *prometheusRenderer) startServer() error {
	ln, err := listenTCP(r.addr)
	if err != nil {
		return err
	}
	go func() { _ = r.server.Serve(ln) }()
	return nil
}

func (r *prometheusRenderer) RenderTrace(trace model.RoutingTrace) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	event := trace.Event
	exchange := event.ExchangeName
	if exchange == "" {
		exchange = "(unknown-exchange)"
	}
	eventType := strings.ToLower(event.EventType)
	if eventType == "" {
		eventType = "unknown"
	}

	r.eventsByExchangeType[exchangeEventKey{exchange, eventType}]++

	destinations := trace.Destinations
	if len(destinations) == 0 {
		destinations = event.Destinations
	}
	if len(destinations) == 0 && eventType == "publish" {
		r.unroutedByExchange[exchange]++
	}
	for _, dest := range destinations {
		q := dest.QueueName
		if q == "" {
			q = "(unknown-queue)"
		}
		r.deliveriesByExchangeQ[exchangeQueueKey{exchange, q}]++
	}

	return nil
}

func (r *prometheusRenderer) serveMetrics(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprint(w, r.metricsText())
}

func (r *prometheusRenderer) metricsText() string {
	var b strings.Builder

	b.WriteString("# HELP amqp_routing_events_total Total routing events observed by the inspector.\n")
	b.WriteString("# TYPE amqp_routing_events_total counter\n")

	keys := make([]exchangeEventKey, 0, len(r.eventsByExchangeType))
	for k := range r.eventsByExchangeType {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].exchange != keys[j].exchange {
			return keys[i].exchange < keys[j].exchange
		}
		return keys[i].eventType < keys[j].eventType
	})
	for _, k := range keys {
		fmt.Fprintf(&b, "amqp_routing_events_total{exchange=%q,event_type=%q} %d\n",
			k.exchange, k.eventType, r.eventsByExchangeType[k])
	}

	b.WriteString("# HELP amqp_routing_deliveries_total Total message deliveries to queues.\n")
	b.WriteString("# TYPE amqp_routing_deliveries_total counter\n")

	qkeys := make([]exchangeQueueKey, 0, len(r.deliveriesByExchangeQ))
	for k := range r.deliveriesByExchangeQ {
		qkeys = append(qkeys, k)
	}
	sort.Slice(qkeys, func(i, j int) bool {
		if qkeys[i].exchange != qkeys[j].exchange {
			return qkeys[i].exchange < qkeys[j].exchange
		}
		return qkeys[i].queue < qkeys[j].queue
	})
	for _, k := range qkeys {
		fmt.Fprintf(&b, "amqp_routing_deliveries_total{exchange=%q,queue=%q} %d\n",
			k.exchange, k.queue, r.deliveriesByExchangeQ[k])
	}

	if len(r.unroutedByExchange) > 0 {
		b.WriteString("# HELP amqp_routing_unrouted_total Publish events with no queue destinations (dropped by broker).\n")
		b.WriteString("# TYPE amqp_routing_unrouted_total counter\n")

		exchanges := make([]string, 0, len(r.unroutedByExchange))
		for ex := range r.unroutedByExchange {
			exchanges = append(exchanges, ex)
		}
		sort.Strings(exchanges)
		for _, ex := range exchanges {
			fmt.Fprintf(&b, "amqp_routing_unrouted_total{exchange=%q} %d\n", ex, r.unroutedByExchange[ex])
		}
	}

	return b.String()
}

func (r *prometheusRenderer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true

	if r.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.server.Shutdown(ctx)
	} else {
		// No HTTP server: write metrics text to writer for non-network use (tests).
		_, err := io.WriteString(r.writer, r.metricsText())
		return err
	}

	return nil
}
