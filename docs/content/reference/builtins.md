---
title: Verbs & builtins
description: The unprefixed language verbs — assert, sleep, wait_until, background — and the closed assert-operator set.
weight: 40
---

Language builtins are unprefixed: they are the scenario *language* (control
and assertion), not capabilities of any provider.

## assert

Kind: assertion. Exactly **one** operator key per step.

```yaml
- run: assert
  with:
    of: "${.total.value}"
    equals: 1
  desc: "exactly once"
```

| operator | passes when |
|---|---|
| `equals` / `notEquals` | numeric comparison when both sides parse as numbers, else string equality |
| `contains` / `absent` | substring (strings) or membership (lists) — `absent` is the negation |
| `in` | `of` equals any element of the operand list |
| `matches` | the operand regex matches `of` |
| `gt` `lt` `gte` `lte` | numeric comparison |
| `between` | `of` within `[min, max]`, inclusive |

## sleep

Kind: action. Seconds (number).

```yaml
- run: sleep
  with: 50
```

Prefer `wait_until` — sleep is for genuinely time-based waits (a TTL, a
scheduler tick), not for "probably done by now".

## wait_until

Kind: probe. Blocks the timeline on an **observed event**: re-runs a probe
until a condition holds or a timeout expires.

```yaml
- run: wait_until
  with:
    probe:                    # any probe step: run/with/read
      run: http.get
      with:
        path: "/jobs/${.job}"
    read: ".state"            # optional jq over the probe's value
    in: [SUCCESS, FAILED]     # exactly one assert operator (any from the table above)
    timeout: 420              # seconds — required
    interval: 1               # seconds between polls — optional, default 1
```

On success it emits a `gate.observed` event and yields the observed value; on
timeout it fails with the **last observed value** in the message.

## background / stop_background

Kind: action. Run a step concurrently with the timeline — load generators,
log followers:

```yaml
- run: background
  with:
    name: load
    step:
      run: exec.run
      with: "scripts/load.sh"
# ... inject faults while load runs ...
- run: stop_background
  with: load
  as: loadResult
```

`stop_background` cancels the step if still running, waits for it, and yields
its result. The capture (`loadResult`) exists only **after**
`stop_background` — referencing it earlier is a `validate` error (rule 6).
A background step killed by the stop is not a failure; its output becomes the
value.

## sample

Runs a probe repeatedly and aggregates the results — for SLO-style assertions
over a window, not a single reading. `Kind: probe`.

| arg | meaning |
|---|---|
| `probe` | the step to sample (a nested `{ run, with, read }`) |
| `count` | number of calls (use this or `duration`) |
| `duration` | seconds to sample for (use this or `count`) |
| `interval` | seconds between calls (default 0) |

Returns an Observation whose `value` is
`{ n, errors, errorRate, min, max, mean, p50, p95, p99 }` (latencies in ms):

```yaml
- run: sample
  with:
    probe: { run: http.get, with: /checkout/42 }
    duration: 30
    interval: 0.1
  as: load
- run: assert
  with: { of: "${.load.value.errorRate}", lt: 0.01 }
- run: assert
  with: { of: "${.load.value.p99}", lt: 200 }
```

Sampling is sequential (one call at a time). For concurrent load, drive a
generator with `background` and `sample` a separate health probe.
