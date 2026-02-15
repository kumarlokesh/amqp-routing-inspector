package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
)

// Edge represents routing from one exchange to one queue.
type Edge struct {
	FromExchange string
	ToQueue      string
	Label        string
	Count        int
}

type edgeKey struct {
	from  string
	to    string
	label string
}

// Graph stores a routing graph aggregated over multiple traces.
type Graph struct {
	name      string
	exchanges map[string]struct{}
	queues    map[string]struct{}
	edges     map[edgeKey]*Edge
}

// New creates an empty graph.
func New(name string) *Graph {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "routing"
	}

	return &Graph{
		name:      name,
		exchanges: make(map[string]struct{}),
		queues:    make(map[string]struct{}),
		edges:     make(map[edgeKey]*Edge),
	}
}

// AddTrace folds a single trace into graph state.
func (g *Graph) AddTrace(trace model.RoutingTrace) {
	exchange := normalize(trace.Event.ExchangeName, "(unknown-exchange)")
	destinations := trace.Destinations
	if len(destinations) == 0 {
		destinations = trace.Event.Destinations
	}
	if len(destinations) == 0 {
		destinations = []model.QueueDestination{{QueueName: "(unrouted)"}}
	}

	g.exchanges[exchange] = struct{}{}
	for _, destination := range destinations {
		queue := normalize(destination.QueueName, "(unknown-queue)")
		g.queues[queue] = struct{}{}

		label := firstNonEmpty(destination.BindingKey, trace.Event.RoutingKey, "(no-key)")
		key := edgeKey{from: exchange, to: queue, label: label}
		edge, exists := g.edges[key]
		if !exists {
			edge = &Edge{FromExchange: exchange, ToQueue: queue, Label: label}
			g.edges[key] = edge
		}
		edge.Count++
	}
}

// Edges returns a sorted copy of all edges.
func (g *Graph) Edges() []Edge {
	out := make([]Edge, 0, len(g.edges))
	for _, edge := range g.edges {
		out = append(out, *edge)
	}
	
	sort.Slice(out, func(i, j int) bool {
		if out[i].FromExchange != out[j].FromExchange {
			return out[i].FromExchange < out[j].FromExchange
		}
		if out[i].ToQueue != out[j].ToQueue {
			return out[i].ToQueue < out[j].ToQueue
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// DOT renders the graph in Graphviz DOT format.
func (g *Graph) DOT() string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("digraph \"%s\" {\n", escape(g.name)))
	builder.WriteString("  rankdir=LR;\n")
	builder.WriteString("  node [fontname=\"Helvetica\"];\n")

	exchanges := mapKeys(g.exchanges)
	queues := mapKeys(g.queues)

	for _, exchange := range exchanges {
		nodeID := "ex:" + exchange
		builder.WriteString(fmt.Sprintf(
			"  \"%s\" [label=\"exchange: %s\", shape=box, style=\"rounded,filled\", fillcolor=\"#E8F1FF\"];\n",
			escape(nodeID),
			escape(exchange),
		))
	}

	for _, queue := range queues {
		nodeID := "q:" + queue
		builder.WriteString(fmt.Sprintf(
			"  \"%s\" [label=\"queue: %s\", shape=ellipse, style=\"filled\", fillcolor=\"#FFF4E6\"];\n",
			escape(nodeID),
			escape(queue),
		))
	}

	for _, edge := range g.Edges() {
		fromID := "ex:" + edge.FromExchange
		toID := "q:" + edge.ToQueue
		label := fmt.Sprintf("%s (%d)", edge.Label, edge.Count)
		builder.WriteString(fmt.Sprintf(
			"  \"%s\" -> \"%s\" [label=\"%s\"];\n",
			escape(fromID),
			escape(toID),
			escape(label),
		))
	}

	builder.WriteString("}\n")
	return builder.String()
}

func mapKeys[K comparable](in map[K]struct{}) []K {
	out := make([]K, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool {
		return fmt.Sprint(out[i]) < fmt.Sprint(out[j])
	})
	return out
}

func escape(value string) string {
	replacer := strings.NewReplacer(`\\`, `\\\\`, `"`, `\\"`)
	return replacer.Replace(value)
}

func normalize(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
