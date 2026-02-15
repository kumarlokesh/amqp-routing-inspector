# amqp-routing-inspector

Protocol-level routing introspection tool for RabbitMQ.

`amqp-routing-inspector` helps you trace exactly how exchanges route messages to queues by consuming RabbitMQ firehose events and rendering them in human-readable, JSON, or Graphviz DOT format.

## Why this exists

RabbitMQ routing bugs are often hard to explain from queue depth alone. This tool gives a direct stream of routing decisions so you can answer questions like:

- "Why did this message hit queue X?"
- "Why did it get duplicated?"
- "Is my topic binding behaving as expected?"

## Features

- Resilient AMQP connection management with retry/backoff.
- Firehose consumer with auto queue declaration and binding.
- Event parsing with robust header and routing-key fallbacks.
- Exchange/queue filtering.
- Output formats:
  - `cli` (human-readable stream)
  - `json` (machine-readable stream)
  - `dot` (routing graph for Graphviz)
- Graceful shutdown with signal handling.
- Unit tests plus opt-in integration test path.

## Requirements

- Go 1.22+
- RabbitMQ with tracing/firehose support
- Access to the firehose exchange (default `amq.rabbitmq.trace`)

Commonly required plugins:

- `rabbitmq_management`
- `rabbitmq_tracing`
- `rabbitmq_firehose`

## Quick start

### 1) Install dependencies

```bash
go mod tidy
```

### 2) Run locally

```bash
go run ./cmd/amqp-routing-inspector --config configs/config.example.yaml
```

### 3) Enable tracing on RabbitMQ

```bash
rabbitmqctl trace_on -p /
```

## Docker quick start

```bash
make docker-up
docker compose exec rabbitmq rabbitmqctl trace_on -p /
```

Stop:

```bash
make docker-down
```

## CLI usage

```bash
amqp-routing-inspector [flags]
```

Important flags:

- `--config` / `-c` path to YAML config
- `--rabbitmq-url` AMQP endpoint
- `--firehose-exchange` firehose exchange name
- `--output` one of `cli|json|dot`
- `--filter-exchange` exchange filter
- `--filter-queue` queue filter
- `--max-events` stop after N matched events
- `--version` print version and exit

Environment variables are also supported (see `internal/config/config.go` and `config.Usage()`).

## Configuration example

See:

- `configs/config.example.yaml`
- `configs/config.docker.yaml`
- `examples/config.minimal.yaml`

## Output formats

### CLI

One line per matched routing trace with timestamp, exchange, routing key, message id, and destinations.

### JSON

Structured `RoutingTrace` records written as newline-delimited JSON.

### DOT

Graph is accumulated while running and emitted on process shutdown (`Ctrl+C`).
Use with Graphviz:

```bash
dot -Tpng graph.dot -o graph.png
```

## Development

Useful targets:

- `make deps`
- `make fmt`
- `make test`
- `make test-race`
- `make coverage`
- `make run`
- `make run-json`
- `make test-integration`

## Testing

Unit tests:

```bash
go test ./...
```

Integration tests (opt-in):

```bash
AMQP_INTEGRATION=1 go test -tags=integration ./test/integration/...
```

## Project layout

```text
cmd/amqp-routing-inspector   CLI entrypoint
internal/app                 service orchestration
internal/config              config load/validate/env/flags
internal/connection          resilient AMQP connection manager
internal/firehose            firehose consumer abstraction
internal/parser              routing event parser
internal/filter              exchange/queue matcher
internal/graph               routing graph model + DOT
internal/output              renderer implementations
test/integration             integration test suite (build tag: integration)
docs/                        architecture/operations/testing docs
```

## Documentation

- `docs/architecture.md`
- `docs/architecture-diagram.md`
- `docs/operations.md`
- `docs/testing.md`

## License

Apache-2.0
