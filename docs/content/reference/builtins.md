---
title: Verbs & builtins
description: The unprefixed language verbs (assert, sleep, wait_until, background, sample, parallel) and the closed assert-operator set.
weight: 40
---

Language builtins are unprefixed: they are the scenario *language* (control
and assertion), not capabilities of any provider.

## assert

Kind: assertion. Exactly **one** operator key per step.

```yaml
- run: assert
  with:
    of: "${.outputs.total.value}"
    equals: 1
  desc: "exactly once"
```

| operator | passes when |
|---|---|
| `equals` / `notEquals` | numeric comparison when both sides parse as numbers, else string equality; `null` equals only an explicit null, never `""` |
| `contains` / `absent` | substring (strings) or membership (lists); `absent` is the negation; any other `of` type is an error |
| `in` | `of` equals any element of the operand list |
| `matches` | the operand regex matches `of` |
| `gt` `lt` `gte` `lte` | numeric comparison |
| `between` | `of` within `[min, max]`, inclusive; reversed bounds are an error |

## sleep

Kind: action. Seconds (number).

```yaml
- run: sleep
  with: 50
```

Prefer `wait_until`; sleep is for genuinely time-based waits (a TTL, a
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
        path: "/jobs/${.outputs.job}"
    read: ".state"            # optional jq over the probe's value
    in: [SUCCESS, FAILED]     # exactly one assert operator (any from the table above)
    timeout: 420              # seconds (required)
    interval: 1               # seconds between polls (optional, default 1)
```

On success it emits a `gate.observed` event and yields the observed value; on
timeout it fails with the **last observed value** (and the last probe error,
if any) in the message. An observation the operator cannot evaluate yet (a
`null` counter before warm-up) counts as condition-not-met and polling
continues; waiting through the not-ready phase is the verb's purpose.

## background / stop_background

Kind: action. Run a step concurrently with the timeline (load generators,
log followers):

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
`stop_background`; referencing it earlier is a `validate` error (rule 6).
A background step killed by the stop is not a failure; its output becomes the
value. A background that died **on its own** (a missing binary, a connection
refused) fails the `stop_background` step: the load the scenario relied on
never ran. Names are handles: starting a second background under a live name
is an error, and a background step cannot itself start another background.

## sample

Runs a probe repeatedly and aggregates the results, for SLO-style assertions
over a window, not a single reading. `Kind: probe`.

| arg | meaning |
|---|---|
| `probe` | the step to sample (a nested `{ run, with, read }`) |
| `count` | number of calls (use this or `duration`) |
| `duration` | seconds to sample for (use this or `count`) |
| `interval` | seconds between calls (default 0, floored at 0.01 so a window never hot-spins against the probe) |

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
  with: { of: "${.outputs.load.value.errorRate}", lt: 0.01 }
- run: assert
  with: { of: "${.outputs.load.value.p99}", lt: 200 }
```

Sampling is sequential (one call at a time). For concurrent load, drive a
generator with `background` and `sample` a separate health probe.

## parallel

Kind: action. Runs several branches concurrently and waits for all of them (a
barrier join), so a scenario can drive load while a fault is active or inject
several faults at the same instant. Each branch is a full step sequence, so it
may act, probe, assert, carry a `finding:`, and even nest another `parallel`.

```yaml
- run: parallel
  with:
    branches:
      - - run: loadgen.drive            # branch 0: hold load
          with: { rps: 50, for: 30s }
      - - run: toxiproxy.partition      # branch 1: fault, then assert under it
          with: { name: db }
        - run: http.get
          as: resp
        - run: assert
          with: { of: "${.outputs.resp.meta.durationMs}", lt: 800 }
```

Semantics:

- **Barrier join.** Every branch runs to completion; there is no
  sibling cancellation, so outcomes never depend on race timing.
- **Deterministic.** Branch events and results are flushed back in
  branch-index order, so the journal and the verdict are identical run to run.
  Live streaming pauses inside a block; its events surface when it completes.
- **Verdict rollup.** Any failing branch step fails the parallel step. A branch
  `finding:` stays a finding and keeps the scenario green.
- **Captures.** A name bound in a branch is visible to steps after the block. A
  branch cannot reference a sibling branch's capture (concurrent branches have
  no ordering); doing so is a `validate` error (rule 12). When more than one
  branch binds the same name, the highest-indexed branch wins.
- **Nesting** is allowed; a safety cap of 64 concurrent branches applies across
  the whole tree.

This differs from `background`/`stop_background`: a `parallel` block has a fixed
branch set and a defined join point, which makes it more deterministic than the
manual fork/join over named handles.

## repeat

Kind: action. Runs a `do:` sequence a fixed number of `times`. It is count-based
only; there is no duration form, so it never reintroduces wall-clock timing. Its
main use is fault cycling: repeating an inject-then-heal sequence to verify that
recovery survives repeated bounces, not just the first.

| key | meaning |
|---|---|
| `times` | required integer `>= 1`: how many iterations to run |
| `stopOnFail` | optional bool, default `true`: stop at the first failing iteration; set `false` to run all `times` and report the full failure pattern |
| `do` | required non-empty list of steps, run in order each iteration |

```yaml
- run: repeat
  desc: "bounce the cache 5 times"
  with:
    times: 5
    do:
      - { run: docker.kill, with: { service: cache } }
      - { run: docker.start, with: { service: cache } }
      - run: wait_until
        with:
          probe: { run: http.get, with: "http://localhost:8080/health" }
          read: "$meta.status"
          equals: 200
          timeout: 10
```

Semantics:

- **Per-step behavior.** Each inner step runs as a normal step, so actions,
  probes, and assertions behave exactly as they do at the top level, and an
  inner fault in `method` is tracked once per iteration.
- **Captures** carry forward across iterations and hold the last iteration's
  values after the block.
- **Dry-run** runs the body once; the per-step action skips still apply.
- **Nesting** with `parallel` works in both directions.

Two restrictions apply (`validate` rule 13): `finding:` is not allowed on a step
inside `do:`, and a `background` started in the body must be paired with a
`stop_background` for the same name in the same body.
