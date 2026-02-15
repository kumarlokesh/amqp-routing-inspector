//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	parserpkg "github.com/kumarlokesh/amqp-routing-inspector/internal/parser"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestFirehosePublishEventVisible(t *testing.T) {
	if os.Getenv("AMQP_INTEGRATION") != "1" {
		t.Skip("set AMQP_INTEGRATION=1 to run integration tests")
	}

	amqpURL := envOrDefault("AMQP_URL", "amqp://guest:guest@localhost:5672/")
	firehoseExchange := envOrDefault("AMQP_FIREHOSE_EXCHANGE", "amq.rabbitmq.trace")

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		t.Fatalf("dial rabbitmq: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel: %v", err)
	}
	defer func() { _ = ch.Close() }()

	traceQueue, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatalf("declare trace queue: %v", err)
	}
	if err := ch.QueueBind(traceQueue.Name, "#", firehoseExchange, false, nil); err != nil {
		t.Fatalf("bind trace queue to firehose exchange %q: %v", firehoseExchange, err)
	}

	deliveries, err := ch.Consume(traceQueue.Name, "", true, true, false, false, nil)
	if err != nil {
		t.Fatalf("consume trace queue: %v", err)
	}

	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	exchangeName := "inspector_e2e_" + unique
	queueName := "inspector_q_" + unique
	routingKey := "rk." + unique
	messageID := "msg-" + unique

	if err := ch.ExchangeDeclare(exchangeName, "topic", false, true, false, false, nil); err != nil {
		t.Fatalf("declare exchange %q: %v", exchangeName, err)
	}
	declaredQueue, err := ch.QueueDeclare(queueName, false, true, false, false, nil)
	if err != nil {
		t.Fatalf("declare queue %q: %v", queueName, err)
	}
	if err := ch.QueueBind(declaredQueue.Name, routingKey, exchangeName, false, nil); err != nil {
		t.Fatalf("bind queue %q: %v", declaredQueue.Name, err)
	}

	publishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := ch.PublishWithContext(publishCtx, exchangeName, routingKey, false, false, amqp.Publishing{
		MessageId:   messageID,
		Timestamp:   time.Now().UTC(),
		ContentType: "text/plain",
		Body:        []byte("integration-test"),
	}); err != nil {
		t.Fatalf("publish test message: %v", err)
	}

	parser := parserpkg.New()
	timeout := time.After(15 * time.Second)

	for {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting for firehose event exchange=%q routing_key=%q", exchangeName, routingKey)
		case delivery, ok := <-deliveries:
			if !ok {
				t.Fatalf("firehose delivery channel closed before matching event")
			}

			event, err := parser.ParseDelivery(delivery)
			if err != nil {
				continue
			}
			if event.EventType != "publish" {
				continue
			}
			if event.ExchangeName != exchangeName {
				continue
			}
			if event.MessageID != "" && event.MessageID != messageID {
				continue
			}
			if event.RoutingKey != routingKey && !strings.HasSuffix(event.RoutingKey, routingKey) {
				continue
			}

			return
		}
	}
}

func envOrDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return fallback
}
