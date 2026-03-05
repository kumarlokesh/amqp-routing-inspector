package output

import (
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
)

type statsdRenderer struct {
	writer io.Writer // used when conn is nil (tests / no-addr mode)
	conn   net.Conn
}

func newStatsdRenderer(writer io.Writer, addr string) (*statsdRenderer, error) {
	r := &statsdRenderer{writer: writer}
	if addr != "" {
		conn, err := net.Dial("udp", addr)
		if err != nil {
			return nil, fmt.Errorf("dial StatsD %s: %w", addr, err)
		}
		r.conn = conn
	}
	return r, nil
}

func (r *statsdRenderer) RenderTrace(trace model.RoutingTrace) error {
	event := trace.Event
	exchange := sanitizeStatsd(event.ExchangeName)
	if exchange == "" {
		exchange = "unknown"
	}
	eventType := sanitizeStatsd(event.EventType)
	if eventType == "" {
		eventType = "unknown"
	}

	metrics := []string{
		fmt.Sprintf("amqp.routing.events.%s.%s:1|c", exchange, eventType),
	}

	destinations := trace.Destinations
	if len(destinations) == 0 {
		destinations = event.Destinations
	}
	if len(destinations) == 0 && strings.ToLower(event.EventType) == "publish" {
		metrics = append(metrics, fmt.Sprintf("amqp.routing.unrouted.%s:1|c", exchange))
	}
	for _, dest := range destinations {
		q := sanitizeStatsd(dest.QueueName)
		if q == "" {
			q = "unknown"
		}
		metrics = append(metrics, fmt.Sprintf("amqp.routing.deliveries.%s.%s:1|c", exchange, q))
	}

	for _, m := range metrics {
		if err := r.send(m); err != nil {
			return err
		}
	}
	return nil
}

func (r *statsdRenderer) send(metric string) error {
	if r.conn != nil {
		_, err := fmt.Fprintln(r.conn, metric)
		return err
	}
	_, err := fmt.Fprintln(r.writer, metric)
	return err
}

func (r *statsdRenderer) Close() error {
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

// sanitizeStatsd converts a string to a safe StatsD metric segment (alphanumeric + underscore).
func sanitizeStatsd(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
