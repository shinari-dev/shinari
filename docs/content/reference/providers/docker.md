---
title: docker
description: "Lifecycle (up/down) plus process faults (kill, stop, pause, logs) over the docker compose CLI."
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

| verb | kind | args | effect |
|---|---|---|---|
| `up` | action | `services` (list, primary), `wait?`, `profiles?` | `compose up -d --wait` |
| `down` | action | — | `compose down -v --remove-orphans` |
| `kill` | action | `service` (primary) | SIGKILL |
| `stop` | action | `service` | SIGTERM (graceful path) |
| `start` | action | `service` | restart a stopped service |
| `pause` / `unpause` | action | `service` | freeze / thaw the process |
| `logs` | probe | `service` (primary), `tail?`, `since?` | container log text; `tail`/`since` fetch an incremental slice (gate on it with `wait_until`) |
| `ps` | probe | `service?` (primary) | container state from `compose ps --all --format json` |

Relative `composeFiles` paths resolve against the project root.

## Starting a service that is meant to fail

`up` runs `compose up -d --wait`, blocking until every started service is
healthy. To bring up a service that is *supposed* to crash or hang (so you can
assert it fails fast rather than blocks), pass `wait: false` — the `--wait` is
dropped and the step returns once the container is created:

```yaml
- run: docker.up
  with: { services: [worker], wait: false }
```

## Service variants (compose profiles)

To run the same role in different shapes (a baseline worker, a round-robin
worker, a partition-failover worker) keep one compose file and tag each variant
service with a [compose profile](https://docs.docker.com/compose/profiles/), then
select one per scenario with `profiles:`. This stays hermetic and keeps a single
lifecycle owner — there is no per-scenario compose-file swapping or second docker
provider (a scenario can still override `composeFiles` in its own `providers:`
block if it genuinely needs a different stack).

```yaml
- run: docker.up
  with: { profiles: [rr], wait: true }   # → compose --profile rr up -d --wait
```

`down` tears the whole project down regardless of profile.

## Inspecting exit state

`ps` returns the parsed `compose ps --all --format json` output (`--all` so
exited and dead containers still report). With a `service` named it binds that
container's object directly, so `read:`/`capture:` reach `.State`, `.ExitCode`,
and `.Health` without indexing a list; with no service it returns the full list.

```yaml
- run: docker.ps
  with: worker
  capture: { state: ".State", code: ".ExitCode" }
- run: assert                              # crashed clean, did not hang
  with: { of: "${.outputs.state}", equals: "exited" }
- run: assert
  with: { of: "${.outputs.code}", equals: 0 }
```
