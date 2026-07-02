---
title: load
description: "Generate controlled HTTP workload and assert on its degradation percentiles under fault."
weight: 65
---

Workload generation. Drives a fixed request rate at a target for a set duration
and returns the same window statistics as the `sample` builtin, so a scenario
can hold a system under load while a fault is active and assert it stayed fast
enough. This is the input side of a degradation assertion, not a load-testing
tool: there are no ramp profiles or distributed runners, and the HTTP engine
underneath is an implementation detail.

```yaml
providers:
  traffic:
    source: load
    config:
      baseUrl: http://localhost:5678
```

`baseUrl` is prepended to each step's `target`; a step may pass an absolute URL
to override it.

## Verbs

### run (action)

Blocks for `duration` seconds while issuing `rate` requests per second, then
returns the window statistics. Its `effect` is `none`: load is the workload, not
a fault, so it does not trip the degradation-observation rule. A request counts
as an error when the transport fails or the status is `>= 400` and not listed
in `expectStatus` — list the codes the system is *supposed* to shed under
fault (a 503 from a load-shedder is graceful degradation, not an error).

| arg | type | req | description |
|---|---|---|---|
| `target` | string | yes | path or URL to request (primary) |
| `rate` | number | yes | requests per second (`>= 1`) |
| `duration` | number | yes | how long to run, in seconds |
| `method` | string | no | HTTP method (default `GET`) |
| `headers` | map | no | request headers |
| `body` | any | no | request body |
| `expectStatus` | list | no | status codes `>= 400` that do not count as errors |

**Returns** the window stats as the value, `{ n, errors, errorRate, min, max,
mean, p50, p95, p99 }` (latencies in ms), identical in shape to `sample`.
`meta` carries `target` (string), `rate` (number), and `durationSec` (number).
`output` is empty.

```yaml
- run: traffic.run
  with: { target: "/", rate: 50, duration: 6 }
  as: loaded
- run: assert
  with: { of: "${.outputs.loaded.value.p95}", lt: 200 }
```

## Driving load while a fault is active

`load.run` owns no start/stop lifecycle — the language already has one. Start
it under `background`, inject and observe the fault on the timeline, then
`stop_background` cancels the attack and returns the stats collected so far.
`duration` becomes the backstop: the window a forgotten stop cannot outlive.

```yaml
- run: background
  with:
    name: traffic
    step: { run: traffic.run, with: { target: "/", rate: 50, duration: 60 } }
- run: toxi.add_latency
  with: { proxy: api, latencyMs: 300 }
- run: sleep
  with: 5
- run: toxi.clear
  with: api
- run: stop_background
  with: traffic
  as: loaded
- run: assert
  with: { of: "${.outputs.loaded.value.p95}", gt: 250 }
  desc: "the injected delay shows up in p95 under load"
```

Alternatively, when the load window should bound the whole exercise, run it in
one branch of a `parallel` block and inject the fault in another, gating the
injection on an observed event:

```yaml
- run: parallel
  with:
    branches:
      - - run: traffic.run
          with: { target: "/", rate: 50, duration: 6 }
          as: loaded
      - - run: wait_until
          with:
            probe: { run: http.get, with: "http://localhost:5678/" }
            matches: "ok"
            timeout: 30
        - run: netem.delay
          with: { target: api, ms: 300 }
- run: assert
  with: { of: "${.outputs.loaded.value.p95}", gt: 250 }
  desc: "the injected delay shows up in p95 under load"
```

Assert on a tolerance (`gt`/`lt`/`between`), never an exact value: percentiles
over a real load window vary run to run.
