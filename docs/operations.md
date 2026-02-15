# Operations Guide

## RabbitMQ prerequisites

Enable plugins:

- `rabbitmq_management`
- `rabbitmq_tracing`
- `rabbitmq_firehose`

Enable tracing for your vhost (default `/`):

```bash
rabbitmqctl trace_on -p /
```

Without trace mode enabled, the firehose exchange may exist but routing events will not appear.

## Running with Docker Compose

```bash
make docker-up
```

Then enable tracing in the RabbitMQ container:

```bash
docker compose exec rabbitmq rabbitmqctl trace_on -p /
```

To stop:

```bash
make docker-down
```

## Running locally

```bash
make run
```

For JSON stream output:

```bash
make run-json
```

## Key runtime settings

- `rabbitmq_url`: broker endpoint.
- `firehose_exchange`: usually `amq.rabbitmq.trace`.
- `queue_name`: leave empty for ephemeral queue.
- `prefetch`: consumer QoS prefetch.
- `max_events`: 0 means unlimited stream.
- `filter_exchange`, `filter_queue`: selective view.

## Troubleshooting

### No events are displayed

1. Ensure tracing is enabled (`rabbitmqctl trace_on -p /`).
2. Verify firehose plugins are enabled.
3. Confirm the configured vhost and credentials can read the firehose exchange.

### Connection loop/retries

- Check broker reachability from the runtime environment.
- Verify URL format (`amqp://user:pass@host:5672/`).
- Increase `reconnect_max` for noisy environments.

### DOT output is empty

- DOT output is written on shutdown; stop the process (Ctrl+C) to flush final graph.
