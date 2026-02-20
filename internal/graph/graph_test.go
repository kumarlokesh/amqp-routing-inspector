package graph

import (
	"strings"
	"testing"
	"time"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
)

func TestGraphAddTraceAggregatesEdgeCounts(t *testing.T) {
	g := New("routing")
	trace := model.RoutingTrace{
		Event: model.RoutingEvent{
			ExchangeName: "orders",
			RoutingKey:   "order.created",
			Timestamp:    time.Now(),
		},
		Destinations: []model.QueueDestination{{QueueName: "payments.q", BindingKey: "order.created"}},
	}

	g.AddTrace(trace)
	g.AddTrace(trace)

	edges := g.Edges()
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].Count != 2 {
		t.Fatalf("expected edge count 2, got %d", edges[0].Count)
	}
}

func TestGraphAddTraceFallbackUnrouted(t *testing.T) {
	g := New("routing")
	g.AddTrace(model.RoutingTrace{
		Event: model.RoutingEvent{ExchangeName: "audit", RoutingKey: "audit.event", Timestamp: time.Now()},
	})

	edges := g.Edges()
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].ToQueue != "(unrouted)" {
		t.Fatalf("expected unrouted edge, got %q", edges[0].ToQueue)
	}
}

func TestGraphDOTContainsNodesAndEdges(t *testing.T) {
	g := New("routing")
	g.AddTrace(model.RoutingTrace{
		Event:        model.RoutingEvent{ExchangeName: "orders", RoutingKey: "order.created", Timestamp: time.Now()},
		Destinations: []model.QueueDestination{{QueueName: "shipping.q", BindingKey: "order.*"}},
	})

	dot := g.DOT()
	for _, expected := range []string{
		"digraph \"routing\"",
		"exchange: orders",
		"queue: shipping.q",
		"order.* (1)",
	} {
		if !strings.Contains(dot, expected) {
			t.Fatalf("DOT output missing %q\nDOT:\n%s", expected, dot)
		}
	}
}
