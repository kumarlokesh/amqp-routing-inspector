package parser

import (
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestParseDeliveryWithExplicitHeaders(t *testing.T) {
	now := time.Date(2026, 2, 14, 18, 0, 0, 0, time.UTC)
	p := NewWithClock(func() time.Time { return now })

	event, err := p.ParseDelivery(amqp.Delivery{
		RoutingKey: "publish.orders.order.created",
		MessageId:  "msg-1",
		Headers: amqp.Table{
			"exchange_name": "orders",
			"exchange_type": "topic",
			"destinations": []any{
				amqp.Table{"queue_name": "payments.q", "binding_key": "order.created"},
				map[string]any{"queue": "analytics.q"},
				"analytics.q",
			},
		},
	})
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}

	if event.ExchangeName != "orders" {
		t.Fatalf("expected exchange orders, got %q", event.ExchangeName)
	}
	if event.EventType != "publish" {
		t.Fatalf("expected event type publish, got %q", event.EventType)
	}
	if event.Timestamp != now {
		t.Fatalf("expected fallback timestamp %v, got %v", now, event.Timestamp)
	}
	if len(event.Destinations) != 2 {
		t.Fatalf("expected 2 unique destinations, got %d", len(event.Destinations))
	}
}

func TestParseDeliveryFallbacksFromRoutingKey(t *testing.T) {
	p := NewWithClock(func() time.Time { return time.Unix(10, 0).UTC() })

	event, err := p.ParseDelivery(amqp.Delivery{
		RoutingKey: "deliver.billing.invoice.paid",
		Headers:    amqp.Table{},
	})
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}

	if event.ExchangeName != "billing" {
		t.Fatalf("expected exchange billing from routing key, got %q", event.ExchangeName)
	}
	if event.EventType != "deliver" {
		t.Fatalf("expected event type deliver, got %q", event.EventType)
	}
}

func TestParseDeliveryInvalidDestinationType(t *testing.T) {
	p := New()

	_, err := p.ParseDelivery(amqp.Delivery{
		RoutingKey: "publish.orders.created",
		Headers: amqp.Table{
			"destinations": 123,
		},
	})
	if err == nil {
		t.Fatalf("expected error for invalid destinations type")
	}
}

func TestParseDeliveryQueueFallback(t *testing.T) {
	p := New()

	event, err := p.ParseDelivery(amqp.Delivery{
		RoutingKey: "publish.orders.created",
		Headers: amqp.Table{
			"queue_name":  "orders.q",
			"binding_key": "order.created",
		},
	})
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}
	if len(event.Destinations) != 1 {
		t.Fatalf("expected 1 destination, got %d", len(event.Destinations))
	}
	if event.Destinations[0].QueueName != "orders.q" {
		t.Fatalf("unexpected destination queue %q", event.Destinations[0].QueueName)
	}
}

func TestParseDeliveryPreservesDottedExchangeName(t *testing.T) {
	p := NewWithClock(func() time.Time { return time.Unix(11, 0).UTC() })

	event, err := p.ParseDelivery(amqp.Delivery{
		RoutingKey: "publish.orders.events.order.created",
		Headers: amqp.Table{
			"routing_key": "order.created",
		},
	})
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}

	if event.ExchangeName != "orders.events" {
		t.Fatalf("expected dotted exchange name orders.events, got %q", event.ExchangeName)
	}
	if event.RoutingKey != "order.created" {
		t.Fatalf("expected parsed routing key order.created, got %q", event.RoutingKey)
	}
}

func TestParseDeliveryPreservesDottedQueueFromDeliverRoutingKey(t *testing.T) {
	p := New()

	event, err := p.ParseDelivery(amqp.Delivery{
		RoutingKey: "deliver.billing.retry.q.invoice.paid",
		Headers: amqp.Table{
			"routing_key": "invoice.paid",
		},
	})
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}

	if len(event.Destinations) != 1 {
		t.Fatalf("expected 1 destination, got %d", len(event.Destinations))
	}
	if event.Destinations[0].QueueName != "billing.retry.q" {
		t.Fatalf("expected dotted queue name billing.retry.q, got %q", event.Destinations[0].QueueName)
	}
}

func TestParseDeliveryBodyPreview(t *testing.T) {
	p := NewWithConfig(10)

	event, err := p.ParseDelivery(amqp.Delivery{
		RoutingKey: "publish.orders.created",
		Body:       []byte("hello world, this is a long body"),
	})
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}

	if event.BodyPreview != "hello worl" {
		t.Fatalf("expected body preview truncated to 10 bytes, got %q", event.BodyPreview)
	}
}

func TestParseDeliveryBodyPreviewDisabled(t *testing.T) {
	p := New() // maxBodyBytes = 0

	event, err := p.ParseDelivery(amqp.Delivery{
		RoutingKey: "publish.orders.created",
		Body:       []byte("some body content"),
	})
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}

	if event.BodyPreview != "" {
		t.Fatalf("expected empty body preview when disabled, got %q", event.BodyPreview)
	}
}
