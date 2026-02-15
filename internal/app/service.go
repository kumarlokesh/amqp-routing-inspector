package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/config"
	"github.com/kumarlokesh/amqp-routing-inspector/internal/connection"
	"github.com/kumarlokesh/amqp-routing-inspector/internal/filter"
	"github.com/kumarlokesh/amqp-routing-inspector/internal/firehose"
	"github.com/kumarlokesh/amqp-routing-inspector/internal/model"
	"github.com/kumarlokesh/amqp-routing-inspector/internal/output"
	parserpkg "github.com/kumarlokesh/amqp-routing-inspector/internal/parser"
	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	errMaxEventsReached = errors.New("max events reached")
	errReconnectNeeded  = errors.New("reconnect needed")
)

// Connector establishes resilient AMQP connections.
type Connector interface {
	ConnectWithRetry(ctx context.Context) (firehose.AMQPConnection, error)
}

type connectorAdapter struct {
	manager *connection.Manager
}

func (a connectorAdapter) ConnectWithRetry(ctx context.Context) (firehose.AMQPConnection, error) {
	return a.manager.ConnectWithRetry(ctx)
}

// Parser transforms raw AMQP deliveries into canonical routing events.
type Parser interface {
	ParseDelivery(delivery amqp.Delivery) (model.RoutingEvent, error)
}

// Service orchestrates connection, consumption, parsing, filtering and rendering.
type Service struct {
	logger    *log.Logger
	maxEvents int

	connector Connector
	consumer  firehose.DeliveryConsumer
	parser    Parser
	matcher   *filter.Matcher
	renderer  output.Renderer

	matchedEvents int
}

// New constructs a service from runtime configuration.
func New(cfg config.Config, logger *log.Logger, writer io.Writer) (*Service, error) {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	renderer, err := output.NewRenderer(cfg.OutputFormat, writer, cfg.GraphName)
	if err != nil {
		return nil, fmt.Errorf("initialize renderer: %w", err)
	}

	mgr := connection.NewManager(cfg.RabbitMQURL, cfg.ReconnectInitial, cfg.ReconnectMax, logger)
	consumer := firehose.NewConsumer(cfg.FirehoseExchange, cfg.QueueName, cfg.ConsumerTag, cfg.Prefetch, logger)

	return NewWithDeps(
		cfg,
		logger,
		connectorAdapter{manager: mgr},
		consumer,
		renderer,
	)
}

// NewWithDeps creates a service with explicit dependencies (useful for tests).
func NewWithDeps(
	cfg config.Config,
	logger *log.Logger,
	connector Connector,
	consumer firehose.DeliveryConsumer,
	renderer output.Renderer,
) (*Service, error) {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	if connector == nil {
		return nil, errors.New("connector must not be nil")
	}
	if consumer == nil {
		return nil, errors.New("consumer must not be nil")
	}
	if renderer == nil {
		return nil, errors.New("renderer must not be nil")
	}

	return &Service{
		logger:    logger,
		maxEvents: cfg.MaxEvents,

		connector: connector,
		consumer:  consumer,
		parser:    parserpkg.New(),
		matcher:   filter.NewMatcher(cfg.FilterExchange, cfg.FilterQueue),
		renderer:  renderer,
	}, nil
}

// SetParser overrides parser implementation (primarily for tests).
func (s *Service) SetParser(p Parser) {
	if p != nil {
		s.parser = p
	}
}

// Run blocks until interrupted, max-events is reached, or a fatal error occurs.
func (s *Service) Run(ctx context.Context) (runErr error) {
	if ctx == nil {
		ctx = context.Background()
	}

	defer func() {
		if err := s.renderer.Close(); err != nil && runErr == nil {
			runErr = fmt.Errorf("close renderer: %w", err)
		}
	}()

	for {
		if ctx.Err() != nil {
			return nil
		}

		conn, err := s.connector.ConnectWithRetry(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return fmt.Errorf("connect with retry: %w", err)
		}
		if conn == nil {
			return errors.New("connector returned nil connection")
		}

		sessionErr := s.runSession(ctx, firehose.WrapConnection(conn))
		if err := conn.Close(); err != nil {
			s.logger.Printf("close connection error: %v", err)
		}

		switch {
		case sessionErr == nil:
			return nil
		case errors.Is(sessionErr, errMaxEventsReached):
			return nil
		case errors.Is(sessionErr, context.Canceled), errors.Is(sessionErr, context.DeadlineExceeded):
			if ctx.Err() != nil {
				return nil
			}
			continue
		case errors.Is(sessionErr, errReconnectNeeded):
			s.logger.Printf("session interrupted; reconnecting")
			continue
		default:
			return sessionErr
		}
	}
}

func (s *Service) runSession(ctx context.Context, conn firehose.Connection) error {
	deliveries, cleanup, err := s.consumer.Consume(ctx, conn)
	if err != nil {
		return fmt.Errorf("consume firehose: %w", err)
	}
	if cleanup == nil {
		cleanup = func() error { return nil }
	}
	defer func() {
		if err := cleanup(); err != nil {
			s.logger.Printf("consumer cleanup error: %v", err)
		}
	}()

	connClosed := conn.NotifyClose(make(chan *amqp.Error, 1))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case amqpErr, ok := <-connClosed:
			if !ok || amqpErr == nil {
				return errReconnectNeeded
			}
			s.logger.Printf("connection closed: %v", amqpErr)
			return errReconnectNeeded
		case delivery, ok := <-deliveries:
			if !ok {
				return errReconnectNeeded
			}
			if err := s.handleDelivery(delivery); err != nil {
				return err
			}
		}
	}
}

func (s *Service) handleDelivery(delivery amqp.Delivery) error {
	event, err := s.parser.ParseDelivery(delivery)
	if err != nil {
		s.logger.Printf("failed to parse routing delivery routing_key=%q: %v", delivery.RoutingKey, err)
		return nil
	}

	trace := model.RoutingTrace{
		Event:        event,
		Destinations: event.Destinations,
	}

	if !s.matcher.Match(trace) {
		return nil
	}

	if err := s.renderer.RenderTrace(trace); err != nil {
		return fmt.Errorf("render trace: %w", err)
	}

	s.matchedEvents++
	if s.maxEvents > 0 && s.matchedEvents >= s.maxEvents {
		return errMaxEventsReached
	}

	return nil
}
