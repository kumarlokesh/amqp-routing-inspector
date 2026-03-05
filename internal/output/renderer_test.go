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

func TestSummaryRendererWritesSummaryOnClose(t *testing.T) {
	var buf bytes.Buffer
	renderer, err := NewRenderer("summary", &buf, "")
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	traces := []model.RoutingTrace{
		{
			Event: model.RoutingEvent{
				ExchangeName: "orders",
				EventType:    "publish",
				RoutingKey:   "order.created",
				Timestamp:    time.Date(2026, 2, 14, 18, 0, 0, 0, time.UTC),
			},
			Destinations: []model.QueueDestination{{QueueName: "payments.q"}},
		},
		{
			Event: model.RoutingEvent{
				ExchangeName: "orders",
				EventType:    "deliver",
				RoutingKey:   "order.created",
				Timestamp:    time.Date(2026, 2, 14, 18, 0, 0, 0, time.UTC),
			},
			Destinations: []model.QueueDestination{{QueueName: "payments.q"}},
		},
	}

	for _, tr := range traces {
		if err := renderer.RenderTrace(tr); err != nil {
			t.Fatalf("RenderTrace() error = %v", err)
		}
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output before Close(), got %d bytes", buf.Len())
	}

	if err := renderer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	out := buf.String()
	for _, expected := range []string{"=== Routing Summary ===", "Total events: 2", "orders", "payments.q", "order.created"} {
		if !strings.Contains(out, expected) {
			t.Errorf("summary output missing %q\noutput=%s", expected, out)
		}
	}
}

func TestMermaidRendererWritesOnClose(t *testing.T) {
	var buf bytes.Buffer
	renderer, err := NewRenderer("mermaid", &buf, "routing")
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	if err := renderer.RenderTrace(sampleTrace()); err != nil {
		t.Fatalf("RenderTrace() error = %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output before Close(), got %d bytes", buf.Len())
	}

	if err := renderer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "flowchart LR") || !strings.Contains(out, "payments") {
		t.Fatalf("unexpected mermaid output: %s", out)
	}
}

func TestPrometheusRendererWritesMetricsTextOnClose(t *testing.T) {
	var buf bytes.Buffer
	// addr="" → no HTTP server, writes to buf on Close
	r, err := newPrometheusRenderer(&buf, "")
	if err != nil {
		t.Fatalf("newPrometheusRenderer() error = %v", err)
	}

	if err := r.RenderTrace(sampleTrace()); err != nil {
		t.Fatalf("RenderTrace() error = %v", err)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"amqp_routing_events_total",
		`exchange="orders"`,
		"amqp_routing_deliveries_total",
		`queue="payments.q"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prometheus output missing %q\noutput=%s", want, out)
		}
	}
}

func TestStatsdRendererWritesMetricLines(t *testing.T) {
	var buf bytes.Buffer
	// addr="" → no UDP, writes to buf
	r, err := newStatsdRenderer(&buf, "")
	if err != nil {
		t.Fatalf("newStatsdRenderer() error = %v", err)
	}

	if err := r.RenderTrace(sampleTrace()); err != nil {
		t.Fatalf("RenderTrace() error = %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "amqp.routing.events.orders") {
		t.Fatalf("statsd output missing expected metric\noutput=%s", out)
	}
}

func TestCLIRendererShowsBodyPreview(t *testing.T) {
	var buf bytes.Buffer
	renderer, err := NewRenderer("cli", &buf, "")
	if err != nil {
		t.Fatalf("NewRenderer() error = %v", err)
	}

	trace := sampleTrace()
	trace.Event.BodyPreview = "hello world"

	if err := renderer.RenderTrace(trace); err != nil {
		t.Fatalf("RenderTrace() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "body=") || !strings.Contains(out, "hello world") {
		t.Fatalf("cli output should contain body preview\noutput=%s", out)
	}
}

func TestNewRendererRejectsPrometheusWithoutConfig(t *testing.T) {
	// NewRenderer (not NewRendererFromConfig) can't handle prometheus/statsd/web
	if _, err := NewRenderer("prometheus", ioDiscard{}, ""); err == nil {
		t.Fatal("expected error for prometheus via NewRenderer")
	}
}
