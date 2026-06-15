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
| `up` | action | `services` (list, primary) | `compose up -d --wait` |
| `down` | action | — | `compose down -v --remove-orphans` |
| `kill` | action | `service` (primary) | SIGKILL |
| `stop` | action | `service` | SIGTERM (graceful path) |
| `start` | action | `service` | restart a stopped service |
| `pause` / `unpause` | action | `service` | freeze / thaw the process |
| `logs` | probe | `service` (primary), `tail?`, `since?` | container log text; `tail`/`since` fetch an incremental slice (gate on it with `wait_until`) |

Relative `composeFiles` paths resolve against the project root.
