package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kumarlokesh/amqp-routing-inspector/internal/app"
	"github.com/kumarlokesh/amqp-routing-inspector/internal/config"
	"github.com/kumarlokesh/amqp-routing-inspector/pkg/version"
)

func main() {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		if errors.Is(err, config.ErrHelpRequested) {
			fmt.Fprint(os.Stdout, config.Usage())
			return
		}
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(2)
	}

	if cfg.ShowVersion {
		fmt.Fprintln(os.Stdout, version.Version)
		return
	}

	logger := log.New(os.Stderr, "amqp-routing-inspector: ", log.LstdFlags|log.Lmicroseconds)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc, err := app.New(cfg, logger, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap error: %v\n", err)
		os.Exit(1)
	}

	if err := svc.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "runtime error: %v\n", err)
		os.Exit(1)
	}
}
