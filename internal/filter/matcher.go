package filter

import (
	"path"
	"strings"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
)

// Matcher applies exchange/queue/routing-key/event-type filters to routing traces.
type Matcher struct {
	exchange   string
	queue      string
	routingKey string
	eventType  string
}

// NewMatcher builds a matcher from optional filter patterns.
//
// exchange and queue support glob wildcards via path.Match (e.g. "orders.*", "billing-?").
// routingKey supports AMQP topic wildcards: * matches one dot-separated word, # matches zero or more.
// eventType must be "publish", "deliver", or empty (match all).
func NewMatcher(exchange, queue, routingKey, eventType string) *Matcher {
	return &Matcher{
		exchange:   strings.TrimSpace(exchange),
		queue:      strings.TrimSpace(queue),
		routingKey: strings.TrimSpace(routingKey),
		eventType:  strings.ToLower(strings.TrimSpace(eventType)),
	}
}

// Match returns true when the trace passes all configured filters.
func (m *Matcher) Match(trace model.RoutingTrace) bool {
	if m == nil {
		return true
	}

	if m.exchange != "" {
		ok, _ := matchGlob(m.exchange, trace.Event.ExchangeName)
		if !ok {
			return false
		}
	}

	if m.eventType != "" && strings.ToLower(trace.Event.EventType) != m.eventType {
		return false
	}

	if m.routingKey != "" && !matchTopicKey(m.routingKey, trace.Event.RoutingKey) {
		return false
	}

	if m.queue == "" {
		return true
	}

	destinations := trace.Destinations
	if len(destinations) == 0 {
		destinations = trace.Event.Destinations
	}

	for _, dest := range destinations {
		ok, _ := matchGlob(m.queue, dest.QueueName)
		if ok {
			return true
		}
	}

	return false
}

// matchGlob matches a glob pattern against a value using path.Match semantics.
// An empty pattern matches everything. Supported wildcards: * (any sequence), ? (any single char).
func matchGlob(pattern, value string) (bool, error) {
	if pattern == "" {
		return true, nil
	}
	return path.Match(pattern, value)
}

// matchTopicKey matches an AMQP topic pattern against a routing key.
//   - * matches exactly one dot-separated word.
//   - # matches zero or more dot-separated words.
//   - An empty pattern matches everything.
func matchTopicKey(pattern, routingKey string) bool {
	if pattern == "" {
		return true
	}
	patternParts := strings.Split(pattern, ".")
	keyParts := strings.Split(routingKey, ".")
	return topicMatch(patternParts, keyParts)
}

func topicMatch(pattern, key []string) bool {
	if len(pattern) == 0 {
		return len(key) == 0
	}
	if pattern[0] == "#" {
		if len(pattern) == 1 {
			return true // # matches everything remaining (including nothing)
		}
		// Try each possible split point for multi-word #
		for i := 0; i <= len(key); i++ {
			if topicMatch(pattern[1:], key[i:]) {
				return true
			}
		}
		return false
	}
	if len(key) == 0 {
		return false
	}
	if pattern[0] == "*" || pattern[0] == key[0] {
		return topicMatch(pattern[1:], key[1:])
	}
	return false
}
