package firehose

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AMQPConnection is the subset of amqp091-go connection behavior we need.
type AMQPConnection interface {
	Channel() (*amqp.Channel, error)
	NotifyClose(receiver chan *amqp.Error) chan *amqp.Error
	Close() error
}

// Channel is the subset of AMQP channel operations needed by the consumer.
type Channel interface {
	Qos(prefetchCount, prefetchSize int, global bool) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
	Close() error
}

// Connection is the AMQP connection behavior needed by the inspector.
type Connection interface {
	OpenChannel() (Channel, error)
	NotifyClose(receiver chan *amqp.Error) chan *amqp.Error
	Close() error
}

type realConnection struct {
	conn AMQPConnection
}

func (r *realConnection) OpenChannel() (Channel, error) {
	ch, err := r.conn.Channel()
	if err != nil {
		return nil, err
	}
	return &realChannel{ch: ch}, nil
}

func (r *realConnection) NotifyClose(receiver chan *amqp.Error) chan *amqp.Error {
	return r.conn.NotifyClose(receiver)
}

func (r *realConnection) Close() error {
	return r.conn.Close()
}

type realChannel struct {
	ch *amqp.Channel
}

func (r *realChannel) Qos(prefetchCount, prefetchSize int, global bool) error {
	return r.ch.Qos(prefetchCount, prefetchSize, global)
}

func (r *realChannel) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	return r.ch.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
}

func (r *realChannel) QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error {
	return r.ch.QueueBind(name, key, exchange, noWait, args)
}

func (r *realChannel) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	return r.ch.Consume(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
}

func (r *realChannel) Close() error {
	return r.ch.Close()
}

// WrapConnection adapts an AMQP connection into the consumer Connection interface.
func WrapConnection(conn AMQPConnection) Connection {
	if conn == nil {
		return nil
	}
	return &realConnection{conn: conn}
}

// DeliveryConsumer streams deliveries from the configured exchange.
type DeliveryConsumer interface {
	Consume(ctx context.Context, conn Connection) (<-chan amqp.Delivery, func() error, error)
}

// Consumer binds a queue to the firehose exchange and consumes routing metadata deliveries.
type Consumer struct {
	exchange    string
	queueName   string
	consumerTag string
	prefetch    int
	logger      *log.Logger
}

// NewConsumer constructs a consumer with sensible defaults.
func NewConsumer(exchange, queueName, consumerTag string, prefetch int, logger *log.Logger) *Consumer {
	exchange = strings.TrimSpace(exchange)
	if exchange == "" {
		exchange = "amq.rabbitmq.trace"
	}
	if prefetch <= 0 {
		prefetch = 200
	}
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	return &Consumer{
		exchange:    exchange,
		queueName:   strings.TrimSpace(queueName),
		consumerTag: strings.TrimSpace(consumerTag),
		prefetch:    prefetch,
		logger:      logger,
	}
}

// Consume starts delivery consumption and returns a context-aware delivery stream.
func (c *Consumer) Consume(ctx context.Context, conn Connection) (<-chan amqp.Delivery, func() error, error) {
	if conn == nil {
		return nil, nil, errors.New("connection is nil")
	}

	ch, err := conn.OpenChannel()
	if err != nil {
		return nil, nil, fmt.Errorf("open channel: %w", err)
	}

	if err := ch.Qos(c.prefetch, 0, false); err != nil {
		_ = ch.Close()
		return nil, nil, fmt.Errorf("configure qos: %w", err)
	}

	declaredQueue, err := c.declareQueue(ch)
	if err != nil {
		_ = ch.Close()
		return nil, nil, err
	}

	if err := ch.QueueBind(declaredQueue.Name, "#", c.exchange, false, nil); err != nil {
		_ = ch.Close()
		return nil, nil, fmt.Errorf("bind queue %q to exchange %q: %w", declaredQueue.Name, c.exchange, err)
	}

	deliveries, err := ch.Consume(declaredQueue.Name, c.consumerTag, true, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		return nil, nil, fmt.Errorf("consume from queue %q: %w", declaredQueue.Name, err)
	}

	c.logger.Printf("consuming firehose events exchange=%s queue=%s prefetch=%d", c.exchange, declaredQueue.Name, c.prefetch)

	var (
		once       sync.Once
		cleanupErr error
	)

	cleanup := func() error {
		once.Do(func() {
			cleanupErr = ch.Close()
		})
		return cleanupErr
	}

	out := make(chan amqp.Delivery)
	go func() {
		defer close(out)
		defer cleanup()

		for {
			select {
			case <-ctx.Done():
				return
			case delivery, ok := <-deliveries:
				if !ok {
					return
				}
				select {
				case out <- delivery:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, cleanup, nil
}

func (c *Consumer) declareQueue(ch Channel) (amqp.Queue, error) {
	if c.queueName == "" {
		q, err := ch.QueueDeclare("", false, true, true, false, nil)
		if err != nil {
			return amqp.Queue{}, fmt.Errorf("declare ephemeral queue: %w", err)
		}
		return q, nil
	}

	q, err := ch.QueueDeclare(c.queueName, false, true, false, false, nil)
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("declare named queue %q: %w", c.queueName, err)
	}
	return q, nil
}
