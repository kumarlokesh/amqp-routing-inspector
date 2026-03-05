# Live Web UI

`--output web` starts an embedded HTTP server with a live routing graph. Open the address in any modern browser to watch the routing topology build in real time.

```bash
amqp-routing-inspector --output web --web-addr :8080
```

Then open `http://localhost:8080` in your browser.

## Features

- **Live routing graph**: Exchanges and queues appear as nodes; routing events create labeled edges showing routing keys and delivery counts.
- **Event log panel**: The 50 most recent events are listed on the left with exchange, routing key, destination queues, and event type badge (publish/deliver).
- **Auto-reconnect**: The browser automatically reconnects after network interruptions.
- **No installation required**: The web UI is embedded in the binary. No separate server, no CDN dependencies for the server itself (Mermaid.js is loaded from CDN in the browser).
- **Zero runtime dependencies**: Uses Server-Sent Events (SSE) — standard HTTP, no WebSocket library needed.

## Architecture

```
Browser <──SSE──> /events   (ndjson routing traces, streamed)
Browser <───GET──> /        (embedded HTML + JS)
```

The graph is rendered using [Mermaid.js](https://mermaid.js.org/) in the browser. As new events arrive via SSE, the graph incrementally updates and re-renders.

## Docker / Kubernetes

```yaml
services:
  inspector:
    image: amqp-routing-inspector
    command:
      - --output=web
      - --web-addr=:8080
      - --rabbitmq-url=amqp://guest:guest@rabbitmq:5672/
    ports:
      - "8080:8080"
```

Then visit `http://localhost:8080` to see the live routing graph.

## Slow-client Handling

The SSE server uses non-blocking sends with a per-client buffer of 32 events. If a client falls behind (e.g. a slow browser tab), events are dropped for that client rather than blocking the inspector. The graph will catch up on the next event.
