package app

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/config"
	"github.com/kumarlokesh/amqp-routing-inspector/internal/firehose"
	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
	amqp "github.com/rabbitmq/amqp091-go"
)

type fakeConnector struct {
	connections []firehose.AMQPConnection
	index       int
}

func (f *fakeConnector) ConnectWithRetry(context.Context) (firehose.AMQPConnection, error) {
	if f.index >= len(f.connections) {
		return nil, context.Canceled
	}
	conn := f.connections[f.index]
	f.index++
	return conn, nil
}

type amqpConnectionStub struct {
	notify chan *amqp.Error
	closed bool
}

func (c *amqpConnectionStub) Channel() (*amqp.Channel, error) {
	return nil, errors.New("not implemented")
}

func (c *amqpConnectionStub) NotifyClose(receiver chan *amqp.Error) chan *amqp.Error {
	if c.notify != nil {
		return c.notify
	}
	return receiver
}

func (c *amqpConnectionStub) Close() error {
	c.closed = true
	return nil
}

type fakeConsumer struct {
	sessions []<-chan amqp.Delivery
	calls    int
}

func (c *fakeConsumer) Consume(context.Context, firehose.Connection) (<-chan amqp.Delivery, func() error, error) {
	if c.calls >= len(c.sessions) {
		return nil, nil, errors.New("unexpected consume call")
	}
	stream := c.sessions[c.calls]
	c.calls++
	return stream, func() error { return nil }, nil
}

type fakeParser struct {
	events []model.RoutingEvent
	errs   []error
	calls  int
}

func (p *fakeParser) ParseDelivery(amqp.Delivery) (model.RoutingEvent, error) {
	idx := p.calls
	p.calls++

	if idx < len(p.errs) && p.errs[idx] != nil {
		return model.RoutingEvent{}, p.errs[idx]
	}
	if idx < len(p.events) {
		return p.events[idx], nil
	}
	return model.RoutingEvent{ExchangeName: "orders", RoutingKey: "rk", Timestamp: time.Now().UTC()}, nil
}

type collectingRenderer struct {
	traces []model.RoutingTrace
	closed bool
}

func (r *collectingRenderer) RenderTrace(trace model.RoutingTrace) error {
	r.traces = append(r.traces, trace)
	return nil
}

func (r *collectingRenderer) Close() error {
	r.closed = true
	return nil
}

func TestServiceRunStopsAtMaxEvents(t *testing.T) {
	deliveries := make(chan amqp.Delivery, 2)
	deliveries <- amqp.Delivery{RoutingKey: "a"}
	deliveries <- amqp.Delivery{RoutingKey: "b"}
	close(deliveries)

	cfg := config.Default()
	cfg.MaxEvents = 1
	renderer := &collectingRenderer{}

	service, err := NewWithDeps(
		cfg,
		log.New(io.Discard, "", 0),
		&fakeConnector{connections: []firehose.AMQPConnection{&amqpConnectionStub{}}},
		&fakeConsumer{sessions: []<-chan amqp.Delivery{deliveries}},
		renderer,
	)
	if err != nil {
		t.Fatalf("NewWithDeps() error = %v", err)
	}
	service.SetParser(&fakeParser{events: []model.RoutingEvent{{ExchangeName: "orders", RoutingKey: "one", Timestamp: time.Now().UTC()}, {ExchangeName: "orders", RoutingKey: "two", Timestamp: time.Now().UTC()}}})

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(renderer.traces) != 1 {
		t.Fatalf("expected 1 rendered trace, got %d", len(renderer.traces))
	}
	if !renderer.closed {
		t.Fatalf("renderer should be closed")
	}
}

func TestServiceRunReconnectsOnClosedStream(t *testing.T) {
	session1 := make(chan amqp.Delivery)
	close(session1)

	session2 := make(chan amqp.Delivery, 1)
	session2 <- amqp.Delivery{RoutingKey: "event"}
	close(session2)

	cfg := config.Default()
	cfg.MaxEvents = 1
	renderer := &collectingRenderer{}
	connector := &fakeConnector{connections: []firehose.AMQPConnection{&amqpConnectionStub{}, &amqpConnectionStub{}}}
	consumer := &fakeConsumer{sessions: []<-chan amqp.Delivery{session1, session2}}

	service, err := NewWithDeps(cfg, log.New(io.Discard, "", 0), connector, consumer, renderer)
	if err != nil {
		t.Fatalf("NewWithDeps() error = %v", err)
	}
	service.SetParser(&fakeParser{events: []model.RoutingEvent{{ExchangeName: "orders", RoutingKey: "event", Timestamp: time.Now().UTC()}}})

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if connector.index != 2 {
		t.Fatalf("expected 2 connection attempts, got %d", connector.index)
	}
	if len(renderer.traces) != 1 {
		t.Fatalf("expected one rendered trace after reconnect, got %d", len(renderer.traces))
	}
}

func TestServiceRunSkipsParseErrors(t *testing.T) {
	deliveries := make(chan amqp.Delivery, 2)
	deliveries <- amqp.Delivery{RoutingKey: "bad"}
	deliveries <- amqp.Delivery{RoutingKey: "good"}
	close(deliveries)

	cfg := config.Default()
	cfg.MaxEvents = 1
	renderer := &collectingRenderer{}

	service, err := NewWithDeps(
		cfg,
		log.New(io.Discard, "", 0),
		&fakeConnector{connections: []firehose.AMQPConnection{&amqpConnectionStub{}}},
		&fakeConsumer{sessions: []<-chan amqp.Delivery{deliveries}},
		renderer,
	)
	if err != nil {
		t.Fatalf("NewWithDeps() error = %v", err)
	}
	service.SetParser(&fakeParser{
		errs:   []error{errors.New("decode failure"), nil},
		events: []model.RoutingEvent{{}, {ExchangeName: "orders", RoutingKey: "good", Timestamp: time.Now().UTC()}},
	})

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(renderer.traces) != 1 {
		t.Fatalf("expected 1 rendered trace after skipping parser error, got %d", len(renderer.traces))
	}
}
