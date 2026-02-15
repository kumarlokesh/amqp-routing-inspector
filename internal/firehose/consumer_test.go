package firehose

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type fakeConnection struct {
	channel Channel
	err     error
}

func (f *fakeConnection) OpenChannel() (Channel, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.channel, nil
}

func (f *fakeConnection) NotifyClose(receiver chan *amqp.Error) chan *amqp.Error {
	return receiver
}

func (f *fakeConnection) Close() error {
	return nil
}

type fakeChannel struct {
	qosErr          error
	declareErr      error
	bindErr         error
	consumeErr      error
	consumeStream   chan amqp.Delivery
	closeErr        error
	queueDeclared   amqp.Queue
	declaredName    string
	declaredFlags   []bool
	boundQueue      string
	boundKey        string
	boundExchange   string
	qosPrefetch     int
	qosPrefetchSize int
	qosGlobal       bool
}

func (f *fakeChannel) Qos(prefetchCount, prefetchSize int, global bool) error {
	f.qosPrefetch = prefetchCount
	f.qosPrefetchSize = prefetchSize
	f.qosGlobal = global
	return f.qosErr
}

func (f *fakeChannel) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	f.declaredName = name
	f.declaredFlags = []bool{durable, autoDelete, exclusive, noWait}
	if f.declareErr != nil {
		return amqp.Queue{}, f.declareErr
	}
	if f.queueDeclared.Name == "" {
		f.queueDeclared = amqp.Queue{Name: "auto.generated"}
	}
	return f.queueDeclared, nil
}

func (f *fakeChannel) QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error {
	f.boundQueue = name
	f.boundKey = key
	f.boundExchange = exchange
	return f.bindErr
}

func (f *fakeChannel) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	if f.consumeErr != nil {
		return nil, f.consumeErr
	}
	if f.consumeStream == nil {
		f.consumeStream = make(chan amqp.Delivery)
	}
	return f.consumeStream, nil
}

func (f *fakeChannel) Close() error {
	if f.consumeStream != nil {
		func() {
			defer func() {
				_ = recover()
			}()
			close(f.consumeStream)
		}()
		f.consumeStream = nil
	}
	return f.closeErr
}

func TestConsumeDeclaresEphemeralQueueWhenQueueNameMissing(t *testing.T) {
	channel := &fakeChannel{queueDeclared: amqp.Queue{Name: "tmp.queue"}, consumeStream: make(chan amqp.Delivery, 1)}
	channel.consumeStream <- amqp.Delivery{RoutingKey: "publish.orders.created"}
	close(channel.consumeStream)

	consumer := NewConsumer("amq.rabbitmq.trace", "", "inspector", 10, log.New(io.Discard, "", 0))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stream, cleanup, err := consumer.Consume(ctx, &fakeConnection{channel: channel})
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	defer cleanup()

	delivery := <-stream
	if delivery.RoutingKey != "publish.orders.created" {
		t.Fatalf("unexpected delivery routing key: %q", delivery.RoutingKey)
	}
	if channel.declaredName != "" {
		t.Fatalf("expected anonymous queue declaration, got %q", channel.declaredName)
	}
	if channel.boundExchange != "amq.rabbitmq.trace" {
		t.Fatalf("expected bind to trace exchange, got %q", channel.boundExchange)
	}
	if channel.boundKey != "#" {
		t.Fatalf("expected bind key '#', got %q", channel.boundKey)
	}
	if channel.qosPrefetch != 10 || channel.qosPrefetchSize != 0 || channel.qosGlobal {
		t.Fatalf("unexpected qos settings: prefetch=%d size=%d global=%v", channel.qosPrefetch, channel.qosPrefetchSize, channel.qosGlobal)
	}
}

func TestConsumeDeclaresNamedQueueWhenProvided(t *testing.T) {
	channel := &fakeChannel{queueDeclared: amqp.Queue{Name: "trace.queue"}, consumeStream: make(chan amqp.Delivery)}
	consumer := NewConsumer("amq.rabbitmq.trace", "trace.queue", "inspector", 1, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, cleanup, err := consumer.Consume(ctx, &fakeConnection{channel: channel})
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	defer cleanup()

	if channel.declaredName != "trace.queue" {
		t.Fatalf("expected named queue declaration, got %q", channel.declaredName)
	}
	if len(channel.declaredFlags) != 4 || channel.declaredFlags[2] {
		t.Fatalf("expected non-exclusive queue for named declaration")
	}
}

func TestConsumeReturnsErrorOnQosFailure(t *testing.T) {
	expectedErr := errors.New("qos failed")
	channel := &fakeChannel{qosErr: expectedErr}
	consumer := NewConsumer("amq.rabbitmq.trace", "", "", 10, nil)

	if _, _, err := consumer.Consume(context.Background(), &fakeConnection{channel: channel}); err == nil {
		t.Fatalf("expected qos error")
	}
}

func TestConsumeReturnsErrorOnNilConnection(t *testing.T) {
	consumer := NewConsumer("amq.rabbitmq.trace", "", "", 10, nil)
	if _, _, err := consumer.Consume(context.Background(), nil); err == nil {
		t.Fatalf("expected nil connection error")
	}
}
