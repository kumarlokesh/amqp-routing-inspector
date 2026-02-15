# Architecture

`amqp-routing-inspector` is a streaming CLI that observes RabbitMQ firehose events and renders routing decisions in real time.

## High-level flow

1. `connection.Manager` establishes and re-establishes AMQP connections.
2. `firehose.Consumer` opens a channel, configures QoS, declares/binds a queue to the firehose exchange, then streams deliveries.
3. `parser.EventParser` normalizes raw AMQP deliveries into `RoutingEvent`/`RoutingTrace` values.
4. `filter.Matcher` applies optional exchange/queue filters.
5. `output.Renderer` writes traces as CLI lines, JSON, or DOT graph output.

## Component boundaries

### cmd/amqp-routing-inspector

- Parses flags and env-backed config.
- Handles `--help` and `--version`.
- Sets up signal-aware context and starts the application service.

### internal/app

- Coordinates reconnect loops and session lifecycle.
- Treats channel closure and connection closure as reconnect signals.
- Performs graceful renderer shutdown.

### internal/connection

- Encapsulates retry/backoff behavior.
- Reconnect policy is configurable (`reconnect_initial`, `reconnect_max`).

### internal/firehose

- Encapsulates AMQP queue/channel setup details.
- Provides test-friendly interfaces to isolate AMQP behavior from business logic.

### internal/parser

- Parses multiple possible header shapes used by firehose.
- Handles fallback extraction from routing keys when headers are incomplete.
- Deduplicates destinations.

### internal/output + internal/graph

- `cli` and `json` render on each trace.
- `dot` renderer accumulates graph state and flushes final DOT in `Close()`.

## Data model

`RoutingEvent`

- `exchange_name`, `exchange_type`, `routing_key`, `message_id`, `timestamp`, `event_type`

`QueueDestination`

- `queue_name`, `binding_key`

`RoutingTrace`

- `event`, `destinations`

## Sequence sketch

```text
main -> app.Service.Run
  -> connector.ConnectWithRetry
  -> firehose.Consumer.Consume
     -> AMQP channel + queue bind
  -> for delivery in stream:
       parser.ParseDelivery
       matcher.Match
       renderer.RenderTrace
  -> on close/error: reconnect
```

## Reliability notes

- Connection retries use exponential backoff capped by `reconnect_max`.
- Session teardown is defensive (nil-safe cleanup; idempotent close handling).
- Parse errors are logged and skipped to keep the stream alive.
- Context cancellation exits cleanly.
