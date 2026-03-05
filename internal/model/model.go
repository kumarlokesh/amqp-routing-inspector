package model

import "time"

// QueueDestination represents a queue and binding observed in routing.
type QueueDestination struct {
	QueueName  string `json:"queue_name" yaml:"queue_name"`
	BindingKey string `json:"binding_key,omitempty" yaml:"binding_key,omitempty"`
}

// RoutingEvent is the canonical parsed event from RabbitMQ firehose metadata.
type RoutingEvent struct {
	ExchangeName  string             `json:"exchange_name" yaml:"exchange_name"`
	ExchangeType  string             `json:"exchange_type,omitempty" yaml:"exchange_type,omitempty"`
	RoutingKey    string             `json:"routing_key" yaml:"routing_key"`
	MessageID     string             `json:"message_id,omitempty" yaml:"message_id,omitempty"`
	CorrelationID string             `json:"correlation_id,omitempty" yaml:"correlation_id,omitempty"`
	Timestamp     time.Time          `json:"timestamp" yaml:"timestamp"`
	EventType     string             `json:"event_type,omitempty" yaml:"event_type,omitempty"`
	Destinations  []QueueDestination `json:"destinations,omitempty" yaml:"destinations,omitempty"`
	// BodyPreview holds the first N bytes of the message body when --show-body-bytes > 0.
	BodyPreview string `json:"body_preview,omitempty" yaml:"body_preview,omitempty"`
}

// RoutingTrace keeps an event with resolved destinations for reporting.
type RoutingTrace struct {
	Event        RoutingEvent       `json:"event" yaml:"event"`
	Destinations []QueueDestination `json:"destinations" yaml:"destinations"`
}
