---
title: Inject network faults
description: Add latency, blackhole traffic, throttle bandwidth, or partition a service with the toxiproxy provider.
weight: 10
---

**Goal:** degrade or cut the network path to one service, deterministically.

## Prerequisites

Toxiproxy must be *in the request path*: your stack routes traffic through a
Toxiproxy proxy (one per link you want to break), and its admin API is
reachable. Typically the proxy runs as a compose service.

Configure the provider once in `project.yml`:

```yaml
providers:
  toxiproxy:
    config:
      adminUrl: http://localhost:8474
```

## Add latency

```yaml
- run: toxiproxy.add_latency
  with:
    proxy: app-to-db
    latencyMs: 800
    jitterMs: 200
```

## Partition a service

`partition` disables the proxy — connections fail immediately, like a network
split:

```yaml
- run: toxiproxy.partition
  with: app-to-db
```

## Blackhole (connections hang, not fail)

`blackhole` keeps connections open but drops all data — the nastier failure
mode, where clients wait instead of erroring:

```yaml
- run: toxiproxy.blackhole
  with: app-to-db
```

## Throttle bandwidth

```yaml
- run: toxiproxy.bandwidth
  with:
    proxy: app-to-db
    rateKbps: 64
```

## Always reset in teardown

Toxics survive the scenario unless removed. Reset them in `teardown` so the
next scenario starts clean:

```yaml
teardown:
  - run: toxiproxy.reset
  - run: docker.down
```

Remember: an explicit `teardown:` **replaces** the default `docker.down`, so
include it yourself, as above.

## Gate the fault on an observed event

Don't partition "after 5 seconds" — partition *the instant the stream is up*:

```yaml
- run: wait_until
  with:
    probe:
      run: docker.logs
      with: app
    matches: "stream started"
    timeout: 30
- run: toxiproxy.partition
  with: app-to-db
```

See [Verbs & builtins](/reference/builtins/) for the full `wait_until` shape.
