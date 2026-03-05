# Advanced Filters

The inspector supports expressive filter patterns for exchange names, queue names, routing keys, and event types.

## Exchange and Queue Filters (glob)

`--filter-exchange` and `--filter-queue` support **glob patterns** via Go's `path.Match`:

| Pattern | Matches | Does not match |
|---------|---------|----------------|
| `orders` | `orders` | `orders-prod` |
| `orders-*` | `orders-prod`, `orders-staging` | `orders`, `billing-prod` |
| `?rders` | `orders` | `orders-prod` |
| `*` | any value | — |

```bash
# Only events from any exchange starting with "billing"
amqp-routing-inspector --filter-exchange "billing*"

# Only events routed to any queue ending in ".retry"
amqp-routing-inspector --filter-queue "*.retry"
```

## Routing Key Filter (AMQP topic wildcards)

`--filter-routing-key` supports **AMQP topic exchange wildcards**:

| Wildcard | Meaning |
|----------|---------|
| `*` | Matches exactly **one** dot-separated word |
| `#` | Matches **zero or more** dot-separated words |

```bash
# Exact match
amqp-routing-inspector --filter-routing-key "order.created"

# Any single word after "order."
amqp-routing-inspector --filter-routing-key "order.*"
# matches: order.created, order.updated
# does not match: order.created.v2

# Any routing key starting with "order."
amqp-routing-inspector --filter-routing-key "order.#"
# matches: order.created, order.created.v2, order (# = zero words)

# Multi-level patterns
amqp-routing-inspector --filter-routing-key "*.created.#"
# matches: order.created, order.created.v2, payment.created.us.east
```

## Event Type Filter

`--filter-event` restricts output to one event type:

```bash
# Only publishing events (message enters the exchange)
amqp-routing-inspector --filter-event publish

# Only delivery events (message reaches a queue)
amqp-routing-inspector --filter-event deliver
```

## Combining Filters

All filters are combined with AND logic. An event must pass every configured filter.

```bash
# Orders exchange, topic routing keys matching order.*, publish events only
amqp-routing-inspector \
  --filter-exchange "orders" \
  --filter-routing-key "order.*" \
  --filter-event publish
```

## Unrouted Message Warning

When `--warn-unrouted` is set, the inspector logs a warning whenever a `publish` event arrives with no queue destinations — messages silently dropped by the broker.

```bash
amqp-routing-inspector --warn-unrouted
# stderr: WARN unrouted message exchange=orders routing_key=dead.letter message_id=msg-99
```
