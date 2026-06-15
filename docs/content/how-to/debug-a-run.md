---
title: Debug a failing run
description: Keep the stack alive, dry-run the timeline, and replay the event journal.
weight: 50
---

**Goal:** figure out why a scenario failed without re-running blind.

## Keep the stack up

Teardown destroys the evidence. Skip it:

```sh
KEEP_UP=1 ./shinari run worker-killed-mid-task
```

The whole `teardown` section is skipped: containers, toxics, and DNS
overrides stay exactly as the failure left them. Poke at the system, then
clean up manually (`docker compose down -v`).

## Dry-run the timeline

```sh
./shinari run -dry-run worker-killed-mid-task
```

`-dry-run` skips every **action** (anything that mutates: `docker.kill`,
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
