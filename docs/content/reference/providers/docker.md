---
title: docker
description: "Lifecycle (up/down) plus process, network, and resource faults (kill, pause, restart, disconnect, throttle) over the docker compose CLI."
weight: 10
---

Lifecycle and process faults. Drives the `docker compose` CLI. This is the
lifecycle provider: it implements `up`/`down` and powers the default teardown.

```yaml
providers:
  docker:
    config:
      composeFiles: [assets/stack.yml]
      project: chaos-run
```

`composeFiles` lists the compose files to drive (relative paths resolve against
the project root); `project` is the compose project name. Every verb except `ps`
returns the command's trimmed stdout as the value, the untrimmed stdout in
`output`, and an empty `meta`.

The project's resolved [`env:` block](../project/#the-env-block) is forwarded to
every `docker compose` invocation, so a compose file's `${VAR}` interpolation is
sourced from `env:` (and its `.env`/`--env-file` values) without exporting
anything into the shell first. Declare the variable in `env:`; a declared value
overrides an ambient process variable of the same name.

## Verbs

### up (action)

Runs `compose up -d --wait`, blocking until every started service is healthy.
Pass `wait: false` to drop `--wait` (see below). Select compose profiles with
`profiles:`.

| arg | type | req | description |
|---|---|---|---|
| `services` | list | no | services to start; all of them when omitted (primary) |
| `wait` | bool | no | wait for health before returning (default `true`) |
| `profiles` | list | no | compose profiles to activate |

```yaml
- run: docker.up
  with: { services: [api, worker] }
```

### down (action)

Runs `compose down -v --remove-orphans`, tearing the whole project down
regardless of profile. This powers the default teardown.

No args.

```yaml
- run: docker.down
```

### kill / stop (action, outage)

Signals a running container: `kill` sends SIGKILL (abrupt), `stop` sends SIGTERM
(the graceful shutdown path). Both inject an outage.

| arg | type | req | description |
|---|---|---|---|
| `service` | string | yes | the service to signal (primary) |

```yaml
- run: docker.kill
  with: worker
```

### start (action)

Restarts a stopped service.

| arg | type | req | description |
|---|---|---|---|
| `service` | string | yes | the service to start (primary) |

```yaml
- run: docker.start
  with: worker
```

### restart (action, outage)

Bounces a service (stop + start) in one step: the graceful rolling-restart
fault. An outage, since work in flight when the SIGTERM lands is dropped, but
one that heals itself, so the interesting assertions are about what peers
observed during the bounce (retries, failover, no lost writes).

| arg | type | req | description |
|---|---|---|---|
| `service` | string | yes | the service to bounce (primary) |

```yaml
- run: docker.restart
  with: api
```

### pause / unpause (action)

Freezes (`pause`) or thaws (`unpause`) a container's processes with `SIGSTOP`/
`SIGCONT`. `pause` carries `effect: outage`; `unpause` reverts it.

| arg | type | req | description |
|---|---|---|---|
| `service` | string | yes | the service to freeze or thaw (primary) |

```yaml
- run: docker.pause
  with: worker
- run: docker.unpause
  with: worker
```

### disconnect / connect (action)

Partitions a single container at the network layer (`disconnect`, an outage)
and reconnects it (`connect`). The process keeps running and co-located peers
are untouched, so the scenario observes last-known-state behavior and
reconnection on restore: a distinct failure mode from `kill`/`stop`/`pause`.
They target one docker network, defaulting to compose's `<project>_default`
(so `network` is required when no `project` is configured); a multi-network
container is isolated by disconnecting each. `connect` restores the compose
service-name DNS alias, so peers resolve the container again.

| arg | type | req | description |
|---|---|---|---|
| `service` | string | yes | the service to partition or reconnect (primary) |
| `network` | string | no | the docker network (default `<project>_default`) |

```yaml
- run: docker.disconnect
  with: worker
- run: docker.connect
  with: worker
```

### throttle / unthrottle (action)

Caps (`throttle`) or restores (`unthrottle`) a container's CPU via
`docker update --cpus`: resource starvation as a degradation. The process keeps
running and keeps its connections, it just gets slow: a distinct failure mode
from `pause` (frozen) and `kill` (gone). `throttle` carries
`effect: degradation`; `unthrottle` reverts it (`--cpus 0` means "no limit").

CPU only, deliberately: a memory ceiling cannot be reset to unlimited through
`docker update`, so it would be a fault with no restore. Inject memory pressure
by restarting the service with compose-level limits instead.

| arg | type | req | description |
|---|---|---|---|
| `service` | string | yes | the service to cap or restore (primary) |
| `cpus` | number | throttle only | the CPU ceiling (e.g. `0.2` = a fifth of one core) |

```yaml
- run: docker.throttle
  with: { service: worker, cpus: 0.2 }
- run: docker.unthrottle
  with: worker
```

### logs (probe)

Fetches a container's logs. `tail`/`since` fetch an incremental slice, so a
`wait_until` can gate on a log line appearing.

| arg | type | req | description |
|---|---|---|---|
| `service` | string | yes | the service whose logs to read (primary) |
| `tail` | string | no | only the last N lines |
| `since` | string | no | only lines since a timestamp or relative time (e.g. `30s`) |

```yaml
- run: wait_until
  with:
    probe: { run: docker.logs, with: { service: worker, tail: "20" } }
    matches: "rebalanced"
    timeout: 30
```

### ps (probe)

Returns parsed `compose ps --all --format json` (`--all` so exited and dead
containers still report). With a `service` named that matches exactly one
container, it binds that container's object directly, so `read:`/`capture:`
reach `.State`, `.ExitCode`, and `.Health` without indexing a list; with no
service it returns the full list.

| arg | type | req | description |
|---|---|---|---|
| `service` | string | no | one service to inspect; all containers when omitted (primary) |

**Returns** the container object (single match) or the list of objects.
`meta.count` (int) is the number of containers. `output` is the raw JSON.

```yaml
- run: docker.ps
  with: worker
  capture: { state: ".State", code: ".ExitCode" }
- run: assert                              # crashed clean, did not hang
  with: { of: "${.outputs.state}", equals: "exited" }
- run: assert
  with: { of: "${.outputs.code}", equals: 0 }
```

### exec (probe)

Runs a command inside a running container (`compose exec -T <service> sh -c`)
and returns its stdout, so a scenario can read internal runtime state (thread
or fd counts, memory, an in-container metric) and baseline-then-compare it with
the standard assert operators. A probe: it observes, it does not inject a
fault. Keep the command read-only; a fault injected through `exec` belongs on
an action step with an explicit `effect:` override.

| arg | type | req | description |
|---|---|---|---|
| `service` | string | yes | the service to run the command in |
| `command` | string | yes | shell command line; pipes and globs work (primary) |

**Returns** the trimmed stdout as the value.

```yaml
- run: docker.exec
  with: { service: worker, command: "ls /proc/1/task | wc -l" }
  as: threads_before
```

## Starting a service that is meant to fail

`up` runs `compose up -d --wait`, blocking until every started service is
healthy. To bring up a service that is *supposed* to crash or hang (so you can
assert it fails fast rather than blocks), pass `wait: false`: the `--wait` is
dropped and the step returns once the container is created.

```yaml
- run: docker.up
  with: { services: [worker], wait: false }
```

## Service variants (compose profiles)

To run the same role in different shapes (a baseline worker, a round-robin
worker, a partition-failover worker) keep one compose file and tag each variant
service with a [compose profile](https://docs.docker.com/compose/profiles/), then
select one per scenario with `profiles:`. This stays hermetic and keeps a single
lifecycle owner; there is no per-scenario compose-file swapping or second docker
provider (a scenario can still override `composeFiles` in its own `providers:`
block if it genuinely needs a different stack).

```yaml
- run: docker.up
  with: { profiles: [rr], wait: true }   # → compose --profile rr up -d --wait
```

`down` tears the whole project down regardless of profile.
