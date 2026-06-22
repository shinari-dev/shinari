---
title: toxiproxy
description: "Proxy-in-path network faults: latency, packet loss, bandwidth, blackhole, and partition."
weight: 20
---

Proxy-in-path network faults. Drives the Toxiproxy admin API through the
official Go client. Each verb names a `proxy` already configured in Toxiproxy
and adds (or clears) a toxic on it, so the fault sits in the request path
between the client and the backend.

```yaml
providers:
  toxiproxy:
    config:
      adminUrl: http://localhost:8474
```

`adminUrl` is the Toxiproxy admin API. Every verb returns `"ok"` as the value
(`reset` returns `"reset"`), with an empty `output` and `meta`.

## Verbs

### add_latency (action, degradation)

Adds a downstream latency toxic: data is delayed, not dropped.

| arg | type | req | description |
|---|---|---|---|
| `proxy` | string | yes | the configured proxy to slow (primary) |
| `latencyMs` | number | yes | added latency in milliseconds |
| `jitterMs` | number | no | random jitter added to the latency |

```yaml
- run: toxiproxy.add_latency
  with: { proxy: db, latencyMs: 300, jitterMs: 50 }
```

### packet_loss (action, outage)

Drops a fraction of data without closing connections, so calls stall and
time out rather than failing fast.

| arg | type | req | description |
|---|---|---|---|
| `proxy` | string | yes | the configured proxy to degrade (primary) |
| `toxicity` | number | no | fraction of data affected, `0`–`1` (default `1.0`) |

```yaml
- run: toxiproxy.packet_loss
  with: { proxy: db, toxicity: 0.3 }
```

### bandwidth (action, degradation)

Throttles throughput to a fixed rate.

| arg | type | req | description |
|---|---|---|---|
| `proxy` | string | yes | the configured proxy to throttle (primary) |
| `rateKbps` | number | yes | throughput cap in kilobits per second |

```yaml
- run: toxiproxy.bandwidth
  with: { proxy: db, rateKbps: 256 }
```

### blackhole (action, outage)

Drops all data while leaving the socket open, so connections hang.

| arg | type | req | description |
|---|---|---|---|
| `proxy` | string | yes | the configured proxy to black-hole (primary) |

```yaml
- run: toxiproxy.blackhole
  with: db
```

### partition (action, outage)

Disables the proxy entirely, so connections fail fast.

| arg | type | req | description |
|---|---|---|---|
| `proxy` | string | yes | the configured proxy to disable (primary) |

```yaml
- run: toxiproxy.partition
  with: db
```

### reset (action)

Removes all toxics and re-enables every proxy: the recovery verb.

No args.

```yaml
- run: toxiproxy.reset
```
