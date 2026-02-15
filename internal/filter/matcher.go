package filter

import (
	"strings"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
)

// Matcher applies exchange/queue filters to routing traces.
type Matcher struct {
	exchange string
	queue    string
}

// NewMatcher builds a matcher from optional exchange and queue filters.
func NewMatcher(exchange, queue string) *Matcher {
	return &Matcher{
		exchange: strings.TrimSpace(exchange),
		queue:    strings.TrimSpace(queue),
	}
}

// Match returns true when the trace passes configured filters.
func (m *Matcher) Match(trace model.RoutingTrace) bool {
	if m == nil {
		return true
	}

	if m.exchange != "" && trace.Event.ExchangeName != m.exchange {
		return false
	}

	if m.queue == "" {
		return true
	}

	destinations := trace.Destinations
	if len(destinations) == 0 {
		destinations = trace.Event.Destinations
	}

	for _, destination := range destinations {
		if destination.QueueName == m.queue {
			return true
		}
	}

	return false
}
