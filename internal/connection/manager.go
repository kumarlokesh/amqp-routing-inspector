package connection

import (
	"context"
	"io"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// DialFunc dials an AMQP connection.
type DialFunc func(url string) (*amqp.Connection, error)

// Manager handles resilient RabbitMQ connection creation.
type Manager struct {
	url              string
	reconnectInitial time.Duration
	reconnectMax     time.Duration
	dial             DialFunc
	logger           *log.Logger
}

// NewManager creates a new connection manager with reconnect settings.
func NewManager(url string, reconnectInitial, reconnectMax time.Duration, logger *log.Logger) *Manager {
	if reconnectInitial <= 0 {
		reconnectInitial = 1 * time.Second
	}
	if reconnectMax <= 0 {
		reconnectMax = 30 * time.Second
	}
	if reconnectMax < reconnectInitial {
		reconnectMax = reconnectInitial
	}
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	return &Manager{
		url:              url,
		reconnectInitial: reconnectInitial,
		reconnectMax:     reconnectMax,
		dial:             amqp.Dial,
		logger:           logger,
	}
}

// ConnectWithRetry keeps retrying until it can establish a connection or context is canceled.
func (m *Manager) ConnectWithRetry(ctx context.Context) (*amqp.Connection, error) {
	delay := m.reconnectInitial
	attempt := 0

	for {
		conn, err := m.dial(m.url)
		if err == nil {
			if attempt > 0 {
				m.logger.Printf("connection re-established after %d retries", attempt)
			}
			return conn, nil
		}
		attempt++

		m.logger.Printf("connection attempt %d failed: %v; retrying in %s", attempt, err, delay)

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}

		delay = nextBackoff(delay, m.reconnectMax)
	}
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}
