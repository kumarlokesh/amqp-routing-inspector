package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/config"
	"github.com/kumarlokesh/amqp-routing-inspector/internal/graph"
	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
)

// Renderer writes routing traces in a specific format.
type Renderer interface {
	RenderTrace(trace model.RoutingTrace) error
	Close() error
}

// NewRenderer returns a renderer for cli, json, dot, summary, or mermaid outputs.
// For prometheus, statsd, and web formats use NewRendererFromConfig.
func NewRenderer(format string, writer io.Writer, graphName string) (Renderer, error) {
	if writer == nil {
		writer = io.Discard
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "cli":
		return &cliRenderer{writer: writer}, nil
	case "json":
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		return &jsonRenderer{encoder: encoder}, nil
	case "dot":
		return &dotRenderer{writer: writer, graph: graph.New(graphName)}, nil
	case "summary":
		return newSummaryRenderer(writer), nil
	case "mermaid":
		return newMermaidRenderer(writer, graphName), nil
	default:
		return nil, fmt.Errorf("unsupported renderer format %q", format)
	}
}

// NewRendererFromConfig creates a renderer using full Config, supporting all output formats
// including prometheus, statsd, and web which require network addresses.
func NewRendererFromConfig(cfg config.Config, writer io.Writer) (Renderer, error) {
	if writer == nil {
		writer = io.Discard
	}

	switch cfg.OutputFormat {
	case config.FormatCLI:
		return &cliRenderer{writer: writer}, nil
	case config.FormatJSON:
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		return &jsonRenderer{encoder: encoder}, nil
	case config.FormatDOT:
		return &dotRenderer{writer: writer, graph: graph.New(cfg.GraphName)}, nil
	case config.FormatSummary:
		return newSummaryRenderer(writer), nil
	case config.FormatMermaid:
		return newMermaidRenderer(writer, cfg.GraphName), nil
	case config.FormatPrometheus:
		return newPrometheusRenderer(writer, cfg.MetricsAddr)
	case config.FormatStatsd:
		return newStatsdRenderer(writer, cfg.StatsdAddr)
	case config.FormatWeb:
		return newWebRenderer(writer, cfg.WebAddr)
	default:
		return nil, fmt.Errorf("unsupported renderer format %q", cfg.OutputFormat)
	}
}

// ── CLI renderer ──────────────────────────────────────────────────────────────

type cliRenderer struct {
	writer io.Writer
}

func (r *cliRenderer) RenderTrace(trace model.RoutingTrace) error {
	event := trace.Event
	destinations := trace.Destinations
	if len(destinations) == 0 {
		destinations = event.Destinations
	}

	timestamp := event.Timestamp.UTC().Format(time.RFC3339Nano)
	line := fmt.Sprintf(
		"%s exchange=%s type=%s routing_key=%s message_id=%s destinations=%s",
		timestamp,
		emptyFallback(event.ExchangeName, "(unknown-exchange)"),
		emptyFallback(event.ExchangeType, "(unknown-type)"),
		emptyFallback(event.RoutingKey, "(no-routing-key)"),
		emptyFallback(event.MessageID, "(no-message-id)"),
		formatDestinations(destinations),
	)

	if event.BodyPreview != "" {
		line += fmt.Sprintf(" body=%q", event.BodyPreview)
	}

	_, err := fmt.Fprintln(r.writer, line)
	return err
}

func (r *cliRenderer) Close() error { return nil }

// ── JSON renderer ─────────────────────────────────────────────────────────────

type jsonRenderer struct {
	encoder *json.Encoder
}

func (r *jsonRenderer) RenderTrace(trace model.RoutingTrace) error {
	return r.encoder.Encode(trace)
}

func (r *jsonRenderer) Close() error { return nil }

// ── DOT renderer ──────────────────────────────────────────────────────────────

type dotRenderer struct {
	writer io.Writer
	graph  *graph.Graph

	mu     sync.Mutex
	closed bool
}

func (r *dotRenderer) RenderTrace(trace model.RoutingTrace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("dot renderer is closed")
	}
	r.graph.AddTrace(trace)
	return nil
}

func (r *dotRenderer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	_, err := io.WriteString(r.writer, r.graph.DOT())
	return err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func formatDestinations(destinations []model.QueueDestination) string {
	if len(destinations) == 0 {
		return "(none)"
	}

	parts := make([]string, 0, len(destinations))
	for _, destination := range destinations {
		queue := emptyFallback(destination.QueueName, "(unknown-queue)")
		if strings.TrimSpace(destination.BindingKey) == "" {
			parts = append(parts, queue)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s(%s)", queue, destination.BindingKey))
	}

	return strings.Join(parts, ",")
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
