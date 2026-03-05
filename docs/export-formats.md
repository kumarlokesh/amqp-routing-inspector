# Export Formats

## Mermaid Output

`--output mermaid` generates a [Mermaid](https://mermaid.js.org/) flowchart diagram of the observed routing topology. Unlike DOT, Mermaid diagrams render directly inside GitHub Markdown, Notion, and most modern documentation platforms.

```bash
# Capture routing topology over 200 events
amqp-routing-inspector --output mermaid --max-events 200 > routing.md
```

Wrap in a fenced code block with `mermaid` for GitHub rendering:

````md
```mermaid
flowchart LR
  ex_orders["exchange: orders"]
  q_payments_q(["queue: payments.q"])
  ex_orders -- "order.created (42)" --> q_payments_q
```
````

Edge labels show `routing_key (count)`, making it easy to spot high-traffic routes.

## Message Body Preview

`--show-body-bytes N` includes the first N bytes of each message's body in the output. This is useful for debugging message formats without a full message consumer.

```bash
# Show first 128 bytes of each message body in CLI output
amqp-routing-inspector --show-body-bytes 128

# Include body preview in JSON output for structured processing
amqp-routing-inspector --output json --show-body-bytes 256
```

**CLI output example:**

```
2026-03-05T10:00:01Z exchange=orders type=topic routing_key=order.created ... body="{\"order_id\":\"ord-42\","
```

**JSON output example:**

```json
{"event":{"exchange_name":"orders","routing_key":"order.created","body_preview":"{\"order_id\":\"ord-42\","},...}
```

> **Note**: Body preview is stored as a UTF-8 string. Binary payloads will appear garbled — use JSON output and process the `body_preview` field if you need binary-safe handling.

## DOT Output (existing)

```bash
amqp-routing-inspector --output dot --max-events 500 | dot -Tsvg > routing.svg
```

See existing documentation for DOT format details.
