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

`expect: closed` inverts the probe for the fault window: the port being
unreachable is then the *passing* observation ("during the partition this
port must be closed"), and an accepting port fails the step.

| arg | type | req | description |
|---|---|---|---|
| `addr` | string | no | target `host:port`; defaults to the configured target (primary) |
| `host` | string | no | target host, paired with `port` instead of `addr` |
| `port` | number | no | target port, paired with `host` |
| `timeout` | number | no | connect deadline in seconds (default 5) |
| `expect` | string | no | `open` (default) or `closed` |

**Returns** whether the port is open (bool). `meta.connectMs` (int) is the
connect latency and `meta.addr` (string) is the target dialed; when the dial
failed, `meta.reason` names how (`refused` \| `timeout` \| `dns` \| `error`),
on the passing `expect: closed` path and the failing default path alike.
`output` is `"connected to <addr> in <n>ms"` or `"<addr> is closed (<reason>)"`.

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

```yaml
method:
  - run: docker.disconnect
    with: cache
  - run: cache.connect            # the outage itself, observed
    with: { expect: closed, timeout: 2 }
    as: down
  - run: assert
    with: { of: "${.outputs.down.meta.reason}", in: [refused, timeout] }
    desc: "the partition drops connections, it does not half-work"
```
