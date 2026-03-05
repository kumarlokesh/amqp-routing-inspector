package parser

import (
	"fmt"
	"strings"
	"time"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
	amqp "github.com/rabbitmq/amqp091-go"
)

// EventParser parses incoming firehose deliveries into routing events.
type EventParser struct {
	now          func() time.Time
	maxBodyBytes int
}

// New creates a parser using time.Now for fallback timestamps.
func New() *EventParser {
	return &EventParser{now: time.Now}
}

// NewWithConfig creates a parser with body preview support.
// maxBodyBytes controls how many bytes of the message body to include (0 disables).
func NewWithConfig(maxBodyBytes int) *EventParser {
	return &EventParser{now: time.Now, maxBodyBytes: maxBodyBytes}
}

// NewWithClock creates a parser with deterministic clock source (for tests).
func NewWithClock(now func() time.Time) *EventParser {
	if now == nil {
		now = time.Now
	}
	return &EventParser{now: now}
}

// ParseDelivery extracts routing metadata from a firehose delivery.
func (p *EventParser) ParseDelivery(d amqp.Delivery) (model.RoutingEvent, error) {
	headers := d.Headers
	routingKeyFromHeaders := parseRoutingKey(headers)
	eventType := firstNonEmpty(
		headerString(headers, "event_type", "event-type", "event"),
		parseEventTypeFromRoutingKey(d.RoutingKey),
	)
	routingKey := firstNonEmpty(routingKeyFromHeaders, d.RoutingKey)

	exchangeFromRoutingKey := parseExchangeFromFirehoseRoutingKey(d.RoutingKey, eventType, routingKeyFromHeaders)

	event := model.RoutingEvent{
		ExchangeName: firstNonEmpty(
			headerString(headers, "exchange_name", "exchange", "x-exchange", "exchange-name"),
			exchangeFromRoutingKey,
		),
		ExchangeType:  headerString(headers, "exchange_type", "exchange-type", "x-exchange-type"),
		RoutingKey:    routingKey,
		MessageID:     firstNonEmpty(d.MessageId, headerString(headers, "message_id", "message-id")),
		CorrelationID: firstNonEmpty(d.CorrelationId, headerString(headers, "correlation_id", "correlation-id")),
		Timestamp:     d.Timestamp,
		EventType:     eventType,
	}

	if event.ExchangeName == "" {
		event.ExchangeName = "(unknown-exchange)"
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = p.now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}

	destinations, err := parseDestinations(headers, eventType, d.RoutingKey, routingKeyFromHeaders)
	if err != nil {
		return model.RoutingEvent{}, fmt.Errorf("parse destinations: %w", err)
	}
	event.Destinations = dedupeDestinations(destinations)

	if p.maxBodyBytes > 0 && len(d.Body) > 0 {
		body := d.Body
		if len(body) > p.maxBodyBytes {
			body = body[:p.maxBodyBytes]
		}
		event.BodyPreview = string(body)
	}

	return event, nil
}

func parseDestinations(headers amqp.Table, eventType, firehoseRoutingKey, parsedRoutingKey string) ([]model.QueueDestination, error) {
	if headers == nil {
		if eventType == "deliver" {
			if queue := parseDeliverQueueFromFirehoseRoutingKey(firehoseRoutingKey, parsedRoutingKey); queue != "" {
				return []model.QueueDestination{{QueueName: queue}}, nil
			}
		}
		return nil, nil
	}

	if raw, ok := headers["destinations"]; ok {
		return parseDestinationList(raw)
	}

	queueName := headerString(headers, "queue_name", "queue", "destination_queue")
	if queueName == "" {
		if eventType == "deliver" {
			if queue := parseDeliverQueueFromFirehoseRoutingKey(firehoseRoutingKey, parsedRoutingKey); queue != "" {
				return []model.QueueDestination{{QueueName: queue}}, nil
			}
		}
		return nil, nil
	}

	return []model.QueueDestination{{
		QueueName:  queueName,
		BindingKey: headerString(headers, "binding_key", "binding-key"),
	}}, nil
}

func parseRoutingKey(headers amqp.Table) string {
	routingKey := headerString(headers, "routing_key", "routing-key")
	if routingKey != "" {
		return routingKey
	}

	if headers == nil {
		return ""
	}

	rawRoutingKeys, ok := headers["routing_keys"]
	if !ok {
		return ""
	}

	return parseStringSliceFirst(rawRoutingKeys)
}

func parseStringSliceFirst(value any) string {
	switch typed := value.(type) {
	case []any:
		for _, entry := range typed {
			if parsed := strings.TrimSpace(stringFromAny(entry)); parsed != "" {
				return parsed
			}
		}
	case []string:
		for _, entry := range typed {
			if parsed := strings.TrimSpace(entry); parsed != "" {
				return parsed
			}
		}
	}

	return ""
}

func parseDestinationList(value any) ([]model.QueueDestination, error) {
	switch typed := value.(type) {
	case []any:
		out := make([]model.QueueDestination, 0, len(typed))
		for _, entry := range typed {
			destination, err := parseDestinationEntry(entry)
			if err != nil {
				return nil, err
			}
			if destination.QueueName != "" {
				out = append(out, destination)
			}
		}
		return out, nil
	case []string:
		out := make([]model.QueueDestination, 0, len(typed))
		for _, queue := range typed {
			if queue != "" {
				out = append(out, model.QueueDestination{QueueName: strings.TrimSpace(queue)})
			}
		}
		return out, nil
	case string:
		queue := strings.TrimSpace(typed)
		if queue == "" {
			return nil, nil
		}
		return []model.QueueDestination{{QueueName: queue}}, nil
	default:
		return nil, fmt.Errorf("unsupported destinations type %T", value)
	}
}

func parseDestinationEntry(value any) (model.QueueDestination, error) {
	switch typed := value.(type) {
	case nil:
		return model.QueueDestination{}, nil
	case amqp.Table:
		return model.QueueDestination{
			QueueName:  firstNonEmpty(stringFromAny(typed["queue_name"]), stringFromAny(typed["queue"])),
			BindingKey: firstNonEmpty(stringFromAny(typed["binding_key"]), stringFromAny(typed["binding-key"])),
		}, nil
	case map[string]any:
		return model.QueueDestination{
			QueueName:  firstNonEmpty(stringFromAny(typed["queue_name"]), stringFromAny(typed["queue"])),
			BindingKey: firstNonEmpty(stringFromAny(typed["binding_key"]), stringFromAny(typed["binding-key"])),
		}, nil
	case string:
		return model.QueueDestination{QueueName: strings.TrimSpace(typed)}, nil
	default:
		return model.QueueDestination{}, fmt.Errorf("unsupported destination entry type %T", value)
	}
}

func headerString(headers amqp.Table, keys ...string) string {
	if headers == nil {
		return ""
	}
	for _, key := range keys {
		if value, ok := headers[key]; ok {
			if s := strings.TrimSpace(stringFromAny(value)); s != "" {
				return s
			}
		}
	}
	return ""
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}

	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func parseExchangeFromFirehoseRoutingKey(firehoseRoutingKey, eventType, parsedRoutingKey string) string {
	firehoseRoutingKey = strings.TrimSpace(firehoseRoutingKey)
	eventType = strings.TrimSpace(eventType)
	parsedRoutingKey = strings.TrimSpace(parsedRoutingKey)

	if firehoseRoutingKey == "" {
		return ""
	}

	if eventType == "" {
		parts := strings.Split(firehoseRoutingKey, ".")
		if len(parts) < 2 {
			return ""
		}
		return strings.TrimSpace(parts[1])
	}

	prefix := eventType + "."
	if !strings.HasPrefix(firehoseRoutingKey, prefix) {
		parts := strings.Split(firehoseRoutingKey, ".")
		if len(parts) < 2 {
			return ""
		}
		return strings.TrimSpace(parts[1])
	}

	remainder := strings.TrimSpace(strings.TrimPrefix(firehoseRoutingKey, prefix))
	if remainder == "" {
		return ""
	}

	if parsedRoutingKey != "" {
		suffix := "." + parsedRoutingKey
		if strings.HasSuffix(remainder, suffix) {
			candidate := strings.TrimSpace(strings.TrimSuffix(remainder, suffix))
			if candidate != "" {
				return candidate
			}
		}
	}

	if eventType == "publish" {
		return remainder
	}

	parts := strings.Split(remainder, ".")
	return strings.TrimSpace(parts[0])
}

func parseDeliverQueueFromFirehoseRoutingKey(firehoseRoutingKey, parsedRoutingKey string) string {
	remainder := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(firehoseRoutingKey), "deliver."))
	if remainder == "" {
		return ""
	}

	parsedRoutingKey = strings.TrimSpace(parsedRoutingKey)
	if parsedRoutingKey != "" {
		suffix := "." + parsedRoutingKey
		if strings.HasSuffix(remainder, suffix) {
			candidate := strings.TrimSpace(strings.TrimSuffix(remainder, suffix))
			if candidate != "" {
				return candidate
			}
		}
	}

	return remainder
}

func parseEventTypeFromRoutingKey(routingKey string) string {
	parts := strings.Split(strings.TrimSpace(routingKey), ".")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func dedupeDestinations(destinations []model.QueueDestination) []model.QueueDestination {
	seen := make(map[string]struct{}, len(destinations))
	result := make([]model.QueueDestination, 0, len(destinations))
	for _, destination := range destinations {
		key := destination.QueueName + "|" + destination.BindingKey
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, destination)
	}
	return result
}
