---
title: redis
description: "Drive and observe a Redis cache: probes to assert on, actions to drive workload, over go-redis."
weight: 52
---

Drive and observe. Probes a Redis cache to assert on (`ping`, `get`, `exists`,
`info`) and drives workload against it (`set`, `del`, `cmd`), over
[go-redis](https://github.com/redis/go-redis). The cache outage or latency
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

## Verbs

### ping (probe)

Round-trips a `PING`. A connection failure is a probe failure, so `ping` works
as a steadyState gate.

No args.

**Returns** `true`. `output` is `"PONG"`. `meta` is empty.

```yaml
steadyState:
  - run: cache.ping
```

### get (probe)

Reads one key. A cache miss is a normal observation, not a failure, so a
scenario can assert a key is gone after an outage.

| arg | type | req | description |
|---|---|---|---|
| `key` | string | yes | the key to read (primary) |

**Returns** the value as a string on a hit, or `null` on a miss, with `meta.hit`
(bool) telling the two apart. `output` is the value, or `"(nil)"` on a miss.

```yaml
- run: cache.get
  with: "session:42"
  as: session
- run: assert
  with: { of: "${.outputs.session.value}", equals: null }
  desc: "cache miss during the partition, not an error"
```

### set (action)

Writes one key, with an optional TTL.

| arg | type | req | description |
|---|---|---|---|
| `key` | string | yes | the key to write (primary) |
| `value` | any | yes | the value to store |
| `ex` | number | no | TTL in seconds |

**Returns** `true`. `output` is `"OK"`. `meta` is empty.

```yaml
- run: cache.set
  with: { key: "session:42", value: "active", ex: 60 }
```

### del (action)

Deletes one or more keys.

| arg | type | req | description |
|---|---|---|---|
| `keys` | list | yes | the keys to delete; a scalar is accepted for one (primary) |

**Returns** the number of keys deleted (int), also in `meta.deleted`. `output`
is `"deleted <n>"`.

```yaml
- run: cache.del
  with: ["session:42", "session:43"]
```

### exists (probe)

Counts how many of the given keys are present.

| arg | type | req | description |
|---|---|---|---|
| `keys` | list | yes | the keys to check; a scalar is accepted for one (primary) |

**Returns** the count present (int). `output` is `"<n> present"`. `meta` is
empty.

```yaml
- run: cache.exists
  with: "session:42"
  as: present
- run: assert
  with: { of: "${.outputs.present.value}", equals: 0 }
  desc: "the key expired during the outage"
```

### info (probe)

Scrapes `INFO` and parses it into a field map.

| arg | type | req | description |
|---|---|---|---|
| `section` | string | no | one INFO section (`replication`, `clients`, …); all sections when omitted (primary) |

**Returns** the parsed `field -> value` map as the value (read `.role`,
`.connected_clients`, and so on). `output` is the raw INFO text. `meta` is empty.

```yaml
- run: cache.info
  with: replication
  read: ".role"
  as: role
- run: assert
  with: { of: "${.outputs.role.value}", equals: "master" }
```

### cmd (action)

The generic escape hatch for any command go-redis does not wrap. Stays a
non-fault by default; set `effect: outage` on the step when the command injects
one (`CLIENT KILL`, `DEBUG SLEEP`, `SHUTDOWN`).

| arg | type | req | description |
|---|---|---|---|
| `args` | list | yes | the command and its arguments, e.g. `[INCR, counter]` (primary) |

**Returns** the command result as the value (a `[]byte` reply becomes a string).
`output` is its string form. `meta` is empty.

```yaml
- run: cache.cmd
  with: ["INCR", "requests"]
  as: n

- run: cache.cmd                    # a fault injected through cmd
  with: ["CLIENT", "KILL", "TYPE", "normal"]
  effect: outage
```
