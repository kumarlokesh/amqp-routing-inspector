package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
)

func sampleTrace() model.RoutingTrace {
	return model.RoutingTrace{
		Event: model.RoutingEvent{
			ExchangeName: "orders",
			ExchangeType: "topic",
			RoutingKey:   "order.created",
			MessageID:    "msg-123",
			Timestamp:    time.Date(2026, 2, 14, 18, 30, 0, 0, time.UTC),
		},
		Destinations: []model.QueueDestination{{QueueName: "payments.q", BindingKey: "order.*"}},
	}
}

func TestCLIRendererWritesHumanReadableLine(t *testing.T) {
	var buf bytes.Buffer
	renderer, err := NewRenderer("cli", &buf, "")
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	if err := renderer.RenderTrace(sampleTrace()); err != nil {
		t.Fatalf("RenderTrace() error = %v", err)
	}

	output := buf.String()
	for _, expected := range []string{"exchange=orders", "routing_key=order.created", "payments.q(order.*)"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("cli output missing %q\noutput=%s", expected, output)
		}
	}
}

func TestJSONRendererWritesValidJSONLine(t *testing.T) {
	var buf bytes.Buffer
	renderer, err := NewRenderer("json", &buf, "")
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	if err := renderer.RenderTrace(sampleTrace()); err != nil {
		t.Fatalf("RenderTrace() error = %v", err)
	}

	var parsed model.RoutingTrace
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("json unmarshal error: %v", err)
	}
	if parsed.Event.ExchangeName != "orders" {
		t.Fatalf("unexpected exchange %q", parsed.Event.ExchangeName)
	}
}

func TestDOTRendererWritesOnClose(t *testing.T) {
	var buf bytes.Buffer
	renderer, err := NewRenderer("dot", &buf, "routing")
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	if err := renderer.RenderTrace(sampleTrace()); err != nil {
		t.Fatalf("RenderTrace() error = %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output before close, got %d bytes", buf.Len())
	}

	if err := renderer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "digraph") || !strings.Contains(output, "payments.q") {
		t.Fatalf("unexpected DOT output: %s", output)
	}
}

func TestNewRendererRejectsUnknownFormat(t *testing.T) {
	if _, err := NewRenderer("xml", ioDiscard{}, ""); err == nil {
		t.Fatalf("expected error for unknown format")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
