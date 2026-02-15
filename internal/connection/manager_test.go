package connection

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestConnectWithRetryRetriesAndSucceeds(t *testing.T) {
	attempts := 0
	mgr := &Manager{
		url:              "amqp://example",
		reconnectInitial: 1 * time.Millisecond,
		reconnectMax:     4 * time.Millisecond,
		logger:           log.New(io.Discard, "", 0),
		dial: func(string) (*amqp.Connection, error) {
			attempts++
			if attempts < 3 {
				return nil, errors.New("temporary dial failure")
			}
			return &amqp.Connection{}, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn, err := mgr.ConnectWithRetry(ctx)
	if err != nil {
		t.Fatalf("ConnectWithRetry() error = %v", err)
	}
	if conn == nil {
		t.Fatalf("expected connection, got nil")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 dial attempts, got %d", attempts)
	}
}

func TestConnectWithRetryContextCancel(t *testing.T) {
	mgr := &Manager{
		url:              "amqp://example",
		reconnectInitial: 5 * time.Millisecond,
		reconnectMax:     10 * time.Millisecond,
		logger:           log.New(io.Discard, "", 0),
		dial: func(string) (*amqp.Connection, error) {
			return nil, errors.New("always fails")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, err := mgr.ConnectWithRetry(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestNextBackoff(t *testing.T) {
	if got := nextBackoff(1*time.Second, 30*time.Second); got != 2*time.Second {
		t.Fatalf("expected 2s, got %s", got)
	}
	if got := nextBackoff(20*time.Second, 30*time.Second); got != 30*time.Second {
		t.Fatalf("expected cap at 30s, got %s", got)
	}
}
