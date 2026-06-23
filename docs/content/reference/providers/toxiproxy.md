---
title: toxiproxy
description: "Proxy-in-path network faults: latency, packet loss, bandwidth, blackhole, timeout, and partition, each scoped to a stream direction."
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

## Direction

A proxied connection is bidirectional, and the toxic verbs (`add_latency`,
`packet_loss`, `bandwidth`, `blackhole`, `timeout`) take an optional `direction`
choosing which leg they degrade:

| value | leg | use |
|---|---|---|
| `from_server` | service to client (default) | responses, jobs, and keepalives flowing to the client |
| `to_server` | client to service | requests, and the path a worker uses to send results back |
| `both` | each leg | installs the toxic on both streams |

`from_server` preserves the original single-direction behavior, so existing
scenarios are unaffected. The toxiproxy-native names `downstream` and `upstream`
are accepted as aliases of `from_server` and `to_server`. Faulting only
`to_server` slows or cuts the result-send path while responses keep flowing, so
the peer is never marked dead. `partition` and `reset` act on the whole
connection and ignore `direction`.

## Verbs

### add_latency (action, degradation)

Adds a downstream latency toxic: data is delayed, not dropped.

| arg | type | req | description |
|---|---|---|---|
| `proxy` | string | yes | the configured proxy to slow (primary) |
| `latencyMs` | number | yes | added latency in milliseconds |
| `jitterMs` | number | no | random jitter added to the latency |
| `direction` | string | no | which leg to slow (see [Direction](#direction), default `from_server`) |

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
| `direction` | string | no | which leg to degrade (see [Direction](#direction), default `from_server`) |

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
| `direction` | string | no | which leg to throttle (see [Direction](#direction), default `from_server`) |

```yaml
- run: toxiproxy.bandwidth
  with: { proxy: db, rateKbps: 256 }
```

### blackhole (action, outage)

Drops all data while leaving the socket open, so connections hang.

| arg | type | req | description |
|---|---|---|---|
| `proxy` | string | yes | the configured proxy to black-hole (primary) |
| `direction` | string | no | which leg to black-hole (see [Direction](#direction), default `from_server`) |

```yaml
- run: toxiproxy.blackhole
  with: db
```

To hold a result in flight and then drop it without tripping dead-peer
detection, latency then blackhole the `to_server` leg while responses keep
flowing:

```yaml
- run: toxiproxy.add_latency
  with: { proxy: backend, latencyMs: 5000, direction: to_server }
- run: toxiproxy.blackhole
  with: { proxy: backend, direction: to_server }
```

### timeout (action, outage)

Drops all data and then closes the connection after the interval, so a call
wedges and is torn down after a bounded wait. This differs from `blackhole`,
which holds the socket open indefinitely.

| arg | type | req | description |
|---|---|---|---|
| `proxy` | string | yes | the configured proxy to wedge (primary) |
| `timeoutMs` | number | yes | wait before the connection is closed, in milliseconds |
| `direction` | string | no | which leg to wedge (see [Direction](#direction), default `from_server`) |

```yaml
- run: toxiproxy.timeout
  with: { proxy: db, timeoutMs: 2000 }
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

### clear (action)

Scoped recovery: removes this proxy's toxics and re-enables it (undoing a
`partition`), leaving every other proxy untouched. Use it to lift a fault from
one connection without disturbing the rest of the run.

| arg | type | req | description |
|---|---|---|---|
| `proxy` | string | yes | the configured proxy to restore (primary) |

```yaml
- run: toxiproxy.clear
  with: db
```

### reset (action)

Global recovery: removes all toxics and re-enables every proxy. Unlike `clear`,
it acts on every proxy at once.

No args.

```yaml
- run: toxiproxy.reset
```
