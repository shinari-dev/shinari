---
title: Drive the docker stack
description: Configure the compose lifecycle, kill processes mid-flight, and read container logs as probes.
weight: 60
---

**Goal:** use the `docker` provider as both the stack lifecycle and a process
fault arsenal.

## Configure once

```yaml
providers:
  docker:
    config:
      composeFiles: [assets/stack.yml]
      project: chaos-run
```

Compose file paths resolve against the **project root**, not the process cwd.
The `docker` provider is the *lifecycle provider* (it implements `up`/`down`).
Exactly one provider may hold that role per scenario, and it powers the
default teardown.

## Bring services up, and wait for health

```yaml
setup:
  - run: docker.up
    with: [postgres, app, worker-a]
```

`docker.up` runs `compose up -d --wait`: it returns when health checks pass,
so give your services `healthcheck:` blocks and `setup` stays race-free.

## Process faults

```yaml
- run: docker.kill      # SIGKILL, no goodbye
  with: worker-a
- run: docker.stop      # SIGTERM, graceful shutdown path
  with: worker-a
- run: docker.pause     # SIGSTOP-like freeze: alive but unresponsive
  with: worker-a
- run: docker.unpause
  with: worker-a
- run: docker.start     # resurrection
  with: worker-a
```

`kill` versus `stop` is not cosmetic: SIGKILL tests crash recovery, SIGTERM
tests your shutdown hooks. A system can pass one and fail the other.

## Logs as an event gate

`docker.logs` is a probe; combine it with `wait_until` to gate faults on
what the service *says* rather than on time:

```yaml
- run: wait_until
  with:
    probe:
      run: docker.logs
      with: worker-a
    matches: "stream started"
    timeout: 30
- run: docker.kill
  with: worker-a
```

## Teardown semantics

With no `teardown:` section, Shinari runs the lifecycle provider's `down`
(`compose down -v --remove-orphans`). An **explicit `teardown:` replaces that
default**: if you add steps (e.g. `toxiproxy.reset`), add `docker.down`
yourself:

```yaml
teardown:
  - run: toxiproxy.reset
  - run: docker.down
```
