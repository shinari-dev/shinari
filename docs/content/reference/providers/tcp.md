---
title: tcp
description: "A connect/reachability probe: is the port up, and how long does connecting take?"
weight: 45
---

L4 reachability. Dials a TCP address and reports whether the port accepts a
connection and how long that took. It is protocol-agnostic, so it probes any
backend by its socket alone: a cache, a queue, a database, a gRPC service. The
connect latency in `meta` is a degradation lens that needs no application
client.

```yaml
providers:
  cache:
    source: tcp
    config:
      addr: "localhost:6379"   # or host + port below
      # host: localhost
      # port: 6379
      # timeout: 5             # connect deadline in seconds (default 5)
```

The configured target is the default; any step may override it. Give an `addr`
(`host:port`), or a `host` plus `port` pair, and an optional `timeout` connect
deadline in seconds.

## Verbs

### connect (probe)

Dials the address once and times the connect. A reachable port succeeds; a
refused or timed-out connection is a probe failure, so `connect` works as a
steadyState gate and inside `wait_until`.

| arg | type | req | description |
|---|---|---|---|
| `addr` | string | no | target `host:port`; defaults to the configured target (primary) |
| `host` | string | no | target host, paired with `port` instead of `addr` |
| `port` | number | no | target port, paired with `host` |
| `timeout` | number | no | connect deadline in seconds (default 5) |

**Returns** `true`. `meta.connectMs` (int) is the connect latency and
`meta.addr` (string) is the target dialed. `output` is `"connected to <addr> in
<n>ms"`.

```yaml
steadyState:
  - run: cache.connect            # the configured target

verify:
  - run: cache.connect
    with: "db:5432"               # a different address
    as: c
  - run: assert
    with: { of: "${.outputs.c.meta.connectMs}", lt: 50 }
    desc: "connects in under 50ms"
```
