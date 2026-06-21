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

| verb | kind | args | returns |
|---|---|---|---|
| `connect` | probe | `addr` (primary), or `host` + `port`; `timeout` | `value: true`; `meta.connectMs`, `meta.addr` |

The configured target is the default; a step may override it. The `with:` scalar
shorthand binds `addr`:

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

A reachable port returns `value: true`; a refused or timed-out connection is a
probe **failure** (the step fails), so `connect` works as a `steadyState` gate
and inside `wait_until`.
