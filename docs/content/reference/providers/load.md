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

| verb | kind | effect | args |
|---|---|---|---|
| `run` | action | none | `target` (primary), `rate` (req/s, `>= 1`), `duration` (seconds), `method?` (default `GET`), `headers?` (map), `body?` |

`load.run` blocks for `duration` seconds while issuing `rate` requests per
second, then returns `{ n, errors, errorRate, min, max, mean, p50, p95, p99 }`
(latencies in ms), identical in shape to `sample`. A request counts as an error
when the transport fails or the status is `>= 400`. Its `effect` is `none`:
load is the workload, not a fault, so it does not trip the
degradation-observation rule.

`load.run` owns no start/stop lifecycle. To drive load *while* a fault is
active, run it in one branch of a `parallel` block and inject the fault in
another, gating the injection on an observed event:

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
  with: { of: "${.loaded.value.p95}", gt: 250 }
  desc: "the injected delay shows up in p95 under load"
```

Assert on a tolerance (`gt`/`lt`/`between`), never an exact value: percentiles
over a real load window vary run to run.
