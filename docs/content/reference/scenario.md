---
title: Scenario schema
description: Every key of the kind Scenario resource and the exact semantics of its five sections.
weight: 20
---

```yaml
apiVersion: shinari/v1          # required — recognition marker
kind: Scenario                  # required
name: data-loss/worker-killed   # required
description: <text>             # optional

providers: ...                  # optional — per-scenario overrides, merged over the Project's (later wins)
vars:                           # optional — merged over Project vars
  sleepSecs: 30

setup:       [ <step>... ]      # optional
steadyState: [ <step>... ]      # optional
method:      [ <phase>... ]     # optional
verify:      [ <step>... ]      # optional
teardown:    [ <step>... ]      # optional — replaces the default when present
```

## Sections

| section | runs | failure consequence |
|---|---|---|
| `setup` | once, first | scenario `ERRORED`; nothing else runs except teardown |
| `steadyState` | **twice**: before `method` (gate) and after it (recovery check) | gate failure ⇒ `INCONCLUSIVE`; recovery failure ⇒ `FAILED` |
| `method` | ordered phases, each an ordered `steps:` list | first failure stops the timeline ⇒ `FAILED` |
| `verify` | once, at the end — **all** steps run even after failures (cumulative) | any non-finding failure ⇒ `FAILED` |
| `teardown` | **always**, even after ERRORED/FAILED; skipped under `KEEP_UP=1` | recorded, never changes the verdict |

A **phase** is:

```yaml
method:
  - phase: "SIGKILL worker-a; a peer recovers the job"
    steps:
      - run: docker.kill
        with: worker-a
```

There are no do/check/after buckets inside a phase — kind comes from the
verb, and steps interleave acting and observing freely.

## Teardown default

When the `teardown:` key is **absent**, Shinari runs `<lifecycle>.down`
(the one configured provider implementing `up`/`down`). When the key is
**present** — even empty — it replaces that default entirely.

## steadyState contract

Must be idempotent: it runs twice. An *active* probe (generate load, assert
it succeeds) must sample a fresh batch each run. `validate` warns when a
steadyState step resolves to a mutating action (rule 9).

## Interpolation

Each `${...}` is a **jq expression** evaluated over the scope — the same jq used
in `read:`/`capture:`, so there is one expression language. The jq input is the
vars overlaid by the captures (a captured name shadows a var), both top-level
fields: `${.job}` reads a var or capture named `job`, `${.rsp.value.total}`
reaches into a captured object, and full jq is available (`${.total // 0}`,
`${.runs | length}`). A reference that is the entire value (`with: ${.job}`)
preserves the result's type; embedded references stringify, and a jq result of
null renders as empty. Captures are scenario-global, ordered, last-write-wins,
visible across sections.

A step's result is captured as an **Observation envelope** `{value, output,
meta}`: `as: rsp` binds the whole envelope, so the payload is `${.rsp.value}`
and the call's facts are `${.rsp.meta.durationMs}` / `${.rsp.meta.status}`.
`read:` and `capture:` operate on the payload. A per-step `timeout:` (seconds)
fails the step if the verb runs longer.
