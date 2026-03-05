# Observability Integration

## Prometheus Metrics

`--output prometheus` starts an HTTP metrics server compatible with [Prometheus](https://prometheus.io/). The inspector continuously scrapes routing events and exposes counters on `/metrics`.

```bash
amqp-routing-inspector --output prometheus --metrics-addr :9090
```

Point your Prometheus scrape config at the address:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: amqp-routing-inspector
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 15s
```

### Exposed Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `amqp_routing_events_total` | counter | `exchange`, `event_type` | Total routing events observed |
| `amqp_routing_deliveries_total` | counter | `exchange`, `queue` | Total deliveries to queues |
| `amqp_routing_unrouted_total` | counter | `exchange` | Publish events with no queue destinations |

### Example output

```
# HELP amqp_routing_events_total Total routing events observed by the inspector.
# TYPE amqp_routing_events_total counter
amqp_routing_events_total{exchange="orders",event_type="publish"} 512
amqp_routing_events_total{exchange="orders",event_type="deliver"} 488

# HELP amqp_routing_deliveries_total Total message deliveries to queues.
# TYPE amqp_routing_deliveries_total counter
amqp_routing_deliveries_total{exchange="orders",queue="payments.q"} 488

# HELP amqp_routing_unrouted_total Publish events with no queue destinations (dropped by broker).
# TYPE amqp_routing_unrouted_total counter
amqp_routing_unrouted_total{exchange="orders"} 3
```

### Grafana Dashboard

With these counters you can build dashboards for:
- **Routing throughput**: `rate(amqp_routing_events_total[1m])` grouped by exchange
- **Queue delivery rate**: `rate(amqp_routing_deliveries_total[1m])` grouped by queue
- **Unrouted alert**: alert when `rate(amqp_routing_unrouted_total[5m]) > 0`

## StatsD Metrics

`--output statsd` emits routing metrics as StatsD counters over UDP. Compatible with Datadog, StatsHat, Graphite, and any StatsD-compatible aggregator.

```bash
amqp-routing-inspector --output statsd --statsd-addr localhost:8125
```

### Emitted Metrics

| Metric | Description |
|--------|-------------|
| `amqp.routing.events.{exchange}.{event_type}` | Routing event counter |
| `amqp.routing.deliveries.{exchange}.{queue}` | Queue delivery counter |
| `amqp.routing.unrouted.{exchange}` | Unrouted message counter |

Special characters in exchange and queue names are replaced with `_`.

```
amqp.routing.events.orders.publish:1|c
amqp.routing.deliveries.orders.payments_q:1|c
```

## Docker / Kubernetes

Both Prometheus and StatsD modes are designed for containerized deployment:

```yaml
# docker-compose.yml
services:
  inspector:
    image: amqp-routing-inspector
    command:
      - --output=prometheus
      - --metrics-addr=:9090
      - --rabbitmq-url=amqp://guest:guest@rabbitmq:5672/
    ports:
      - "9090:9090"
```
