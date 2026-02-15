package filter

import (
	"testing"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
)

func TestMatcherNoFilterMatchesAll(t *testing.T) {
	matcher := NewMatcher("", "")
	if !matcher.Match(model.RoutingTrace{Event: model.RoutingEvent{ExchangeName: "orders"}}) {
		t.Fatalf("expected trace to match without filters")
	}
}

func TestMatcherExchangeFilter(t *testing.T) {
	matcher := NewMatcher("orders", "")

	if !matcher.Match(model.RoutingTrace{Event: model.RoutingEvent{ExchangeName: "orders"}}) {
		t.Fatalf("expected exchange orders to match")
	}
	if matcher.Match(model.RoutingTrace{Event: model.RoutingEvent{ExchangeName: "payments"}}) {
		t.Fatalf("expected exchange payments not to match")
	}
}

func TestMatcherQueueFilter(t *testing.T) {
	matcher := NewMatcher("", "orders.q")
	trace := model.RoutingTrace{
		Event: model.RoutingEvent{ExchangeName: "orders"},
		Destinations: []model.QueueDestination{
			{QueueName: "orders.q"},
		},
	}

	if !matcher.Match(trace) {
		t.Fatalf("expected queue filter to match")
	}

	trace.Destinations = []model.QueueDestination{{QueueName: "other.q"}}
	if matcher.Match(trace) {
		t.Fatalf("expected queue filter mismatch")
	}
}
