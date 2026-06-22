---
title: Wait for a service to recover
description: Block until a container is healthy again after an outage, polling its healthcheck instead of sleeping a fixed interval.
weight: 25
---

**Goal:** after a fault, continue the moment a dependency is healthy again, not a guessed number of seconds later.

## The problem with a fixed sleep

After an outage a container is often still running but transiently unhealthy:
its last healthcheck failed while the fault was active, and it needs a few
seconds to pass again. Sleeping a fixed interval is both racy and slow. Too
short and the next step races a half-recovered service; too long and every run
pays for the worst case.

```yaml
# Racy: hopes recovery takes under 10s, wastes time when it takes less.
- run: sleep
  with: 10
```

## Poll the healthcheck instead

`docker.ps` reports a service's `Health` (when the compose service defines a
healthcheck). Drive it with `wait_until`, which re-runs a probe to a deadline
and stops as soon as the condition holds:

```yaml
- run: wait_until
  with:
    probe:
      run: docker.ps
      with: api
    read: .Health
    equals: healthy
    timeout: 60
    interval: 2
```

This polls `api` every 2 seconds and proceeds the instant its healthcheck
reports `healthy`, failing the scenario only if 60 seconds pass first. It works
the same whether the container was just started or was already running and
unhealthy: the wait is a poll, not a one-shot read of a stale status.

## Recover and wait, end to end

A typical recovery sequence restarts the dependency and then waits for it to be
healthy before asserting that in-flight work completed:

```yaml
method:
  - phase: outage
    steps:
      - run: docker.kill
        with: api
  - phase: recover
    steps:
      - run: docker.start
        with: api
      - run: wait_until
        with:
          probe: { run: docker.ps, with: api }
          read: .Health
          equals: healthy
          timeout: 60
verify:
  - run: sut.await
    with: "${.outputs.id}"
```

## `up --wait` versus polling for health

`docker.up` with the default `wait: true` already blocks until services are
healthy, so it covers the initial bring-up in `setup`. Use the `wait_until` plus
`docker.ps` pattern for *recovery*, when the container is already running and you
need to block until its healthcheck passes again rather than start it.

See [Verbs & builtins](/reference/builtins/) for the full `wait_until` shape and
the assert-operator set its condition shares.
