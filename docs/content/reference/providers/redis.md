---
title: redis
description: "Drive and observe a Redis cache: probes to assert on, actions to drive workload, over go-redis."
weight: 52
---

Drive and observe. Probes a Redis cache to assert on (`ping`, `get`, `exists`,
`info`) and drives workload against it (`set`, `del`, `cmd`). A native provider
over [go-redis](https://github.com/redis/go-redis). The cache outage or latency
itself is injected by the fault providers (`net`, `toxiproxy`, `docker`); `redis`
is the workload and observation lens, the same way `sql` pairs with those faults.

```yaml
providers:
  cache:
    source: redis
    config:
      addr: "localhost:6379"   # or url: "redis://user:pass@localhost:6379/0"
      password: ""             # optional
      db: 0                    # optional, default 0
```

Configuration takes either an `addr` (`host:port`) with optional `username`,
`password`, and `db`, or a single `url` (`redis://…`). The client connects
lazily, so Redis does not need to be up until the first verb runs (after
`setup`).

| verb | kind | args |
|---|---|---|
| `ping` | probe | — |
| `get` | probe | `key` (primary) |
| `set` | action | `key` (primary), `value`, `ex?` (TTL in seconds) |
| `del` | action | `keys` (primary, list or scalar) |
| `exists` | probe | `keys` (primary, list or scalar) |
| `info` | probe | `section?` (primary) |
| `cmd` | action | `args` (primary, list) |

`get` returns the value as a string. A cache miss is a normal observation, not a
failure: the missing key returns a `null` value (with `meta.hit: false`) so a
scenario can assert a key is gone after an outage. `set` returns `true`; pass
`ex:` for a TTL. `del` and `exists` return a count and accept a single key or a
list. `info` returns the parsed `field -> value` map (read `.role`,
`.connected_clients`, and so on) with the raw text as output. `cmd` is the
generic escape hatch for any command go-redis does not wrap; it stays a
non-fault by default but a step can set `effect: outage` to declare a fault
injected through it (`CLIENT KILL`, `DEBUG SLEEP`, `SHUTDOWN`).

```yaml
# Prime the cache, cut the link to Redis, assert the read survives the miss.
- run: cache.set
  with: { key: "session:42", value: "active", ex: 60 }
- run: net.partition
  with: redis
- run: cache.get
  with: "session:42"
  as: session
- run: assert
  with: { of: "${.outputs.session.value}", equals: null }
  desc: "cache miss during the partition, not an error"
```
