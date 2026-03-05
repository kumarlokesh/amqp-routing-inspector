package output

import (
	"errors"
	"io"
	"sync"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/graph"
	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
)

type mermaidRenderer struct {
	writer io.Writer
	graph  *graph.Graph

	mu     sync.Mutex
	closed bool
}

func newMermaidRenderer(writer io.Writer, graphName string) *mermaidRenderer {
	return &mermaidRenderer{
		writer: writer,
		graph:  graph.New(graphName),
	}
}

func (r *mermaidRenderer) RenderTrace(trace model.RoutingTrace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("mermaid renderer is closed")
	}
	r.graph.AddTrace(trace)
	return nil
}

func (r *mermaidRenderer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	_, err := io.WriteString(r.writer, r.graph.Mermaid())
	return err
}
