package filter

import (
	"testing"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
)

func makeTrace(exchange, eventType, routingKey string, queues ...string) model.RoutingTrace {
	dests := make([]model.QueueDestination, 0, len(queues))
	for _, q := range queues {
		dests = append(dests, model.QueueDestination{QueueName: q})
	}
	return model.RoutingTrace{
		Event: model.RoutingEvent{
			ExchangeName: exchange,
			EventType:    eventType,
			RoutingKey:   routingKey,
		},
		Destinations: dests,
	}
}

func TestMatcherNoFilterMatchesAll(t *testing.T) {
	m := NewMatcher("", "", "", "")
	if !m.Match(makeTrace("orders", "publish", "order.created", "payments.q")) {
		t.Fatal("expected match with no filters")
	}
}

// ── Exchange filter ───────────────────────────────────────────────────────────

func TestMatcherExchangeExact(t *testing.T) {
	m := NewMatcher("orders", "", "", "")
	if !m.Match(makeTrace("orders", "publish", "rk")) {
		t.Fatal("expected exact exchange match")
	}
	if m.Match(makeTrace("billing", "publish", "rk")) {
		t.Fatal("expected exchange mismatch")
	}
}

func TestMatcherExchangeGlob(t *testing.T) {
	m := NewMatcher("orders-*", "", "", "")

	cases := []struct {
		exchange string
		want     bool
	}{
		{"orders-prod", true},
		{"orders-staging", true},
		{"orders", false},
		{"billing-prod", false},
	}
	for _, tc := range cases {
		got := m.Match(makeTrace(tc.exchange, "publish", "rk"))
		if got != tc.want {
			t.Errorf("exchange=%q: got %v, want %v", tc.exchange, got, tc.want)
		}
	}
}

func TestMatcherExchangeWildcardStar(t *testing.T) {
	m := NewMatcher("*", "", "", "")
	if !m.Match(makeTrace("any-exchange", "publish", "rk")) {
		t.Fatal("* should match any exchange")
	}
}

// ── Queue filter ──────────────────────────────────────────────────────────────

func TestMatcherQueueExact(t *testing.T) {
	m := NewMatcher("", "payments.q", "", "")
	if !m.Match(makeTrace("orders", "publish", "rk", "payments.q")) {
		t.Fatal("expected queue match")
	}
	if m.Match(makeTrace("orders", "publish", "rk", "billing.q")) {
		t.Fatal("expected queue mismatch")
	}
}

func TestMatcherQueueGlob(t *testing.T) {
	m := NewMatcher("", "payments.*", "", "")

	if !m.Match(makeTrace("orders", "publish", "rk", "payments.q")) {
		t.Fatal("glob should match payments.q")
	}
	if m.Match(makeTrace("orders", "publish", "rk", "billing.q")) {
		t.Fatal("glob should not match billing.q")
	}
}

func TestMatcherQueueMatchesAnyDestination(t *testing.T) {
	m := NewMatcher("", "target.q", "", "")
	// Multiple destinations: match if any passes
	trace := model.RoutingTrace{
		Event: model.RoutingEvent{ExchangeName: "orders"},
		Destinations: []model.QueueDestination{
			{QueueName: "other.q"},
			{QueueName: "target.q"},
		},
	}
	if !m.Match(trace) {
		t.Fatal("should match when at least one destination matches")
	}
}

// ── Routing-key filter ────────────────────────────────────────────────────────

func TestMatcherRoutingKeyExact(t *testing.T) {
	m := NewMatcher("", "", "order.created", "")
	if !m.Match(makeTrace("orders", "publish", "order.created")) {
		t.Fatal("exact routing key should match")
	}
	if m.Match(makeTrace("orders", "publish", "order.updated")) {
		t.Fatal("exact routing key should not match different key")
	}
}

func TestMatcherRoutingKeyAMQPSingleWildcard(t *testing.T) {
	m := NewMatcher("", "", "order.*", "")
	cases := []struct {
		key  string
		want bool
	}{
		{"order.created", true},
		{"order.updated", true},
		{"order.created.v2", false}, // * is one word only
		{"billing.created", false},
	}
	for _, tc := range cases {
		got := m.Match(makeTrace("orders", "publish", tc.key))
		if got != tc.want {
			t.Errorf("routing key=%q: got %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestMatcherRoutingKeyAMQPHashWildcard(t *testing.T) {
	m := NewMatcher("", "", "order.#", "")
	cases := []struct {
		key  string
		want bool
	}{
		{"order.created", true},
		{"order.created.v2", true}, // # matches multiple words
		{"order", true},            // # matches zero or more words, so "order.#" matches "order" per AMQP spec
		{"billing.created", false},
	}
	for _, tc := range cases {
		got := m.Match(makeTrace("orders", "publish", tc.key))
		if got != tc.want {
			t.Errorf("routing key=%q: got %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestMatcherRoutingKeyHashMatchesEmpty(t *testing.T) {
	// "#" alone should match any routing key
	m := NewMatcher("", "", "#", "")
	for _, key := range []string{"order.created", "x", "a.b.c.d"} {
		if !m.Match(makeTrace("ex", "publish", key)) {
			t.Errorf("# should match key=%q", key)
		}
	}
}

// ── Event-type filter ─────────────────────────────────────────────────────────

func TestMatcherEventTypePublish(t *testing.T) {
	m := NewMatcher("", "", "", "publish")
	if !m.Match(makeTrace("orders", "publish", "rk")) {
		t.Fatal("publish filter should match publish event")
	}
	if m.Match(makeTrace("orders", "deliver", "rk")) {
		t.Fatal("publish filter should not match deliver event")
	}
}

func TestMatcherEventTypeDeliver(t *testing.T) {
	m := NewMatcher("", "", "", "deliver")
	if !m.Match(makeTrace("orders", "deliver", "rk")) {
		t.Fatal("deliver filter should match deliver event")
	}
	if m.Match(makeTrace("orders", "publish", "rk")) {
		t.Fatal("deliver filter should not match publish event")
	}
}

func TestMatcherEventTypeCaseInsensitive(t *testing.T) {
	m := NewMatcher("", "", "", "PUBLISH")
	if !m.Match(makeTrace("orders", "publish", "rk")) {
		t.Fatal("event type filter should be case-insensitive")
	}
}

// ── Combined filters ──────────────────────────────────────────────────────────

func TestMatcherAllFilters(t *testing.T) {
	m := NewMatcher("orders*", "payments.*", "order.*", "publish")
	match := m.Match(model.RoutingTrace{
		Event: model.RoutingEvent{
			ExchangeName: "orders",
			EventType:    "publish",
			RoutingKey:   "order.created",
		},
		Destinations: []model.QueueDestination{{QueueName: "payments.q"}},
	})
	if !match {
		t.Fatal("all filters should match")
	}
}

func TestMatcherAllFiltersOneFails(t *testing.T) {
	m := NewMatcher("orders", "payments.q", "order.*", "publish")
	// Wrong event type: deliver instead of publish
	match := m.Match(model.RoutingTrace{
		Event: model.RoutingEvent{
			ExchangeName: "orders",
			EventType:    "deliver",
			RoutingKey:   "order.created",
		},
		Destinations: []model.QueueDestination{{QueueName: "payments.q"}},
	})
	if match {
		t.Fatal("should not match when event type filter fails")
	}
}

// ── AMQP topic matching edge cases ────────────────────────────────────────────

func TestTopicMatchHashInMiddle(t *testing.T) {
	cases := []struct {
		pattern, key string
		want         bool
	}{
		{"a.#.b", "a.b", true},
		{"a.#.b", "a.x.b", true},
		{"a.#.b", "a.x.y.b", true},
		{"a.#.b", "a.x.c", false},
		{"#.b", "x.y.b", true},
		{"#.b", "b", true}, // # matches zero words: "#.b" matches "b" per AMQP spec
		{"a.#", "a", true}, // # matches zero words after "a" per AMQP spec
	}
	for _, tc := range cases {
		got := matchTopicKey(tc.pattern, tc.key)
		if got != tc.want {
			t.Errorf("matchTopicKey(%q, %q) = %v, want %v", tc.pattern, tc.key, got, tc.want)
		}
	}
}
