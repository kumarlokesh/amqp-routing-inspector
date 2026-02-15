# Testing Strategy

## Test layers

### Unit tests

Located alongside implementation files under `internal/**`.

Goals:

- Parser correctness across header/routing-key variants.
- Connection retry behavior.
- Firehose consumer setup and lifecycle behavior.
- App service reconnect and max-events behavior.
- Output renderers and graph aggregation.

Run:

```bash
make test
```

Race detector:

```bash
make test-race
```

Coverage report:

```bash
make coverage
```

## Integration tests

Integration tests live under `test/integration` and are tagged with `integration`.
They require a reachable RabbitMQ instance with firehose/tracing enabled.

Run:

```bash
make test-integration
```

Environment variables used by integration tests:

- `AMQP_INTEGRATION=1` (required to opt in)
- `AMQP_URL` (default `amqp://guest:guest@localhost:5672/`)
- `AMQP_FIREHOSE_EXCHANGE` (default `amq.rabbitmq.trace`)

## CI

GitHub Actions workflow (`.github/workflows/ci.yml`) performs:

- dependency download
- gofmt verification
- `go vet ./...`
- `go test -race -coverprofile=coverage.out ./...`

This keeps style, safety checks, and test quality enforced on every push/PR.
