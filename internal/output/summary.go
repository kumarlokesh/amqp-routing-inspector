package output

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
)

type summaryRenderer struct {
	writer    io.Writer
	startTime time.Time

	mu            sync.Mutex
	closed        bool
	totalEvents   int
	publishEvents int
	deliverEvents int
	unrouted      int
	exchanges     map[string]int
	queues        map[string]int
	routingKeys   map[string]int
}

func newSummaryRenderer(writer io.Writer) *summaryRenderer {
	return &summaryRenderer{
		writer:      writer,
		startTime:   time.Now(),
		exchanges:   make(map[string]int),
		queues:      make(map[string]int),
		routingKeys: make(map[string]int),
	}
}

func (r *summaryRenderer) RenderTrace(trace model.RoutingTrace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("summary renderer is closed")
	}

	event := trace.Event
	r.totalEvents++

	switch strings.ToLower(event.EventType) {
	case "publish":
		r.publishEvents++
	case "deliver":
		r.deliverEvents++
	}

	exchange := event.ExchangeName
	if exchange == "" {
		exchange = "(unknown-exchange)"
	}
	r.exchanges[exchange]++

	destinations := trace.Destinations
	if len(destinations) == 0 {
		destinations = event.Destinations
	}
	if len(destinations) == 0 && strings.ToLower(event.EventType) == "publish" {
		r.unrouted++
	}
	for _, dest := range destinations {
		q := dest.QueueName
		if q == "" {
			q = "(unknown-queue)"
		}
		r.queues[q]++
	}

	rk := event.RoutingKey
	if rk != "" {
		r.routingKeys[rk]++
	}

	return nil
}

func (r *summaryRenderer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true

	elapsed := time.Since(r.startTime)
	rate := 0.0
	if elapsed.Seconds() > 0 {
		rate = float64(r.totalEvents) / elapsed.Seconds()
	}

	fmt.Fprintf(r.writer, "\n=== Routing Summary ===\n")
	fmt.Fprintf(r.writer, "Duration:     %s\n", elapsed.Round(time.Millisecond))
	fmt.Fprintf(r.writer, "Total events: %d  (publish: %d  deliver: %d  rate: %.1f/s)\n",
		r.totalEvents, r.publishEvents, r.deliverEvents, rate)
	if r.unrouted > 0 {
		fmt.Fprintf(r.writer, "Unrouted:     %d\n", r.unrouted)
	}

	r.writeTopN(r.writer, "Top Exchanges", r.exchanges, r.totalEvents, 10)
	r.writeTopN(r.writer, "Top Queues", r.queues, r.totalEvents, 10)
	r.writeTopN(r.writer, "Top Routing Keys", r.routingKeys, r.totalEvents, 10)

	return nil
}

type countEntry struct {
	name  string
	count int
}

func (r *summaryRenderer) writeTopN(w io.Writer, header string, counts map[string]int, total, n int) {
	if len(counts) == 0 {
		return
	}

	entries := make([]countEntry, 0, len(counts))
	for name, count := range counts {
		entries = append(entries, countEntry{name, count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].name < entries[j].name
	})

	if len(entries) > n {
		entries = entries[:n]
	}

	fmt.Fprintf(w, "\n%s:\n", header)
	for i, e := range entries {
		pct := 0.0
		if total > 0 {
			pct = float64(e.count) / float64(total) * 100
		}
		fmt.Fprintf(w, "  %2d. %-40s %6d  (%5.1f%%)\n", i+1, e.name, e.count, pct)
	}
}
