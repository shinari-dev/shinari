---
title: Debug a failing run
description: Preview the timeline, keep the stack alive, stream values, and replay the event journal.
weight: 50
---

**Goal:** figure out why a scenario failed without re-running blind.

## Preview what it would do

Before running anything, see the whole timeline statically:

```sh
./shinari explain worker-killed-mid-task
```

`explain` prints the lifecycle (`setup` → `steadyState` → `method` phases →
`verify` → `teardown`) with each step's resolved verb, its kind (`[action]`,
`[probe]`, `[assertion]`), any fault effect, and the `finding` markers. It
executes nothing and touches no system. Use it to confirm the shape of a
scenario, spot a misordered phase, or check which steps are faults before
committing to a run.

## Stream values and durations

```sh
./shinari run --verbose worker-killed-mid-task   # or -v
```

`--verbose` adds section banners and, for every step, the value it produced and
how long it took (`✓ jobstore.status → RUNNING (12ms)`). It turns the console
into a live trace, so a probe returning the wrong value or a step running far
too long shows up inline rather than only in the journal.

## Keep the stack up

Teardown destroys the evidence. Skip it:

```sh
./shinari run --keep-up worker-killed-mid-task
```

The whole `teardown` section is skipped: containers, toxics, and DNS
overrides stay exactly as the failure left them. Poke at the system, then
clean up manually (`docker compose down -v`). The `KEEP_UP=1` environment
variable does the same thing.

## Dry-run the timeline

```sh
./shinari run --dry-run worker-killed-mid-task
```

`--dry-run` skips every **action** (anything that mutates: `docker.kill`,
`http.post`, `exec.run` unless overridden) and still executes probes and
assertions. Use it to check wiring (do the probes resolve, do the captures
flow) without touching the system.

## Read the journal

Every run writes `journal.jsonl`: the complete, ordered event stream, one
JSON object per line.

```sh
jq -r 'select(.type=="step.failed") | "\(.scenario) :: \(.step) :: \(.payload.error)"' shinari-out/journal.jsonl
```

Useful event types: `fault.injected` (what actually got injected, when),
`gate.observed` (what value finally satisfied a `wait_until`),
`finding.recorded`, `step.failed`. Timestamps let you reconstruct the exact
timeline of injection versus observation.

## Watch a single check

Target one scenario by name (or a whole suite by directory name):

```sh
./shinari run worker-killed-mid-task     # one scenario
./shinari run recovery                   # the recovery suite
```

## Common failure signatures

| symptom | likely cause |
|---|---|
| `ERRORED` + compose output | stack didn't come up: read the setup step's error, it embeds stderr |
| `INCONCLUSIVE` | steadyState failed *before* any fault: the environment, not the product |
| `wait_until: condition not observed within Ns; last observed: X` | the gate never fired; `X` tells you what the probe actually saw |
| a finding flipped to `✗ finding now passes` | not a bug: the gap was fixed, promote the assertion |
