---
title: Observing degradation
description: Assert that a fault changed behavior, not just that the system survived it.
weight: 40
---

A resilience test that only checks the system still answers measures the easy
half. The harder half is whether it stayed *fast enough* and *failed
gracefully*. Shinari makes that observable.

Every probe result is an Observation envelope `{value, output, meta}`. `meta`
carries `durationMs` (stamped for every verb) and, for HTTP, `status`. After
`as: rsp` you assert on them by jq path:

```yaml
- run: assert
  with: { of: "${.outputs.rsp.meta.durationMs}", lt: 200 }
- run: assert
  with: { of: "${.outputs.rsp.meta.status}", equals: 503 }   # graceful degradation
```

For behavior over a window rather than a single call, `sample` runs a probe
repeatedly and returns `{ n, errors, errorRate, p50, p95, p99, ... }`:

```yaml
- run: sample
  with: { probe: { run: http.get, with: /checkout }, duration: 30 }
  as: load
- run: assert
  with: { of: "${.outputs.load.value.p99}", lt: 200 }
```

`validate` warns (rule 11) when a `degradation` fault is injected but nothing
observes its effect: a survival-only check on a latency fault is usually a
gap, not a pass.
