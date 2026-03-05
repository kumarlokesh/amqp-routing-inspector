# Statistics and Analysis

## Summary Output Format

`--output summary` accumulates all observed events and prints a concise report when the inspector exits (via `--max-events` or Ctrl-C).

```bash
amqp-routing-inspector --output summary --max-events 1000
```

Sample output:

```
=== Routing Summary ===
Duration:     12.453s
Total events: 1,000  (publish: 512  deliver: 488  rate: 80.3/s)
Unrouted:     3

Top Exchanges:
   1. orders                                      423  (42.3%)
   2. billing                                     289  (28.9%)
   3. notifications                               288  (28.8%)

Top Queues:
   1. payments.queue                              389
   2. billing.queue                               289
   3. email.queue                                 288

Top Routing Keys:
   1. order.created                               234  (23.4%)
   2. invoice.paid                                289  (28.9%)
   3. email.send                                  244  (24.4%)
```

The summary is written to stdout, so it can be captured and parsed:

```bash
amqp-routing-inspector --output summary --max-events 500 > report.txt 2>inspector.log
```

## Unrouted Detection

An "unrouted" event is a `publish` event where the broker found no matching queue bindings — the message was silently dropped.

Enable warnings with `--warn-unrouted`:

```bash
amqp-routing-inspector --warn-unrouted
```

Log output (stderr):

```
WARN unrouted message exchange=orders routing_key=order.unknown message_id=msg-42
```

This is useful for:
- Detecting misconfigured topic exchange bindings
- Finding messages that bypass all consumers during topology changes
- Alerting on dropped messages in production

When using `--output summary`, unrouted messages are counted and displayed in the summary.
