---
title: Scenario schema
description: Every key of the kind Scenario resource and the exact semantics of its five sections.
weight: 20
---

```yaml
apiVersion: shinari/v1          # required: recognition marker
kind: Scenario                  # required
name: data-loss/worker-killed   # required
description: <text>             # optional
tags: [slow, network]           # optional: labels for run/list filtering

providers: ...                  # optional: per-scenario overrides, merged over the Project's (later wins)
vars:                           # optional: merged over Project vars
  sleepSecs: 30
timeout: 120                    # optional: whole-scenario deadline in seconds

setup:       [ <step>... ]      # optional
steadyState: [ <step>... ]      # optional
method:      [ <phase>... ]     # optional
verify:      [ <step>... ]      # optional
teardown:    [ <step>... ]      # optional: replaces the default when present
```

## Sections

| section | runs | failure consequence |
|---|---|---|
| `setup` | once, first | scenario `ERRORED`; nothing else runs except teardown |
| `steadyState` | **twice**: before `method` (gate) and after it (recovery check) | gate failure ⇒ `INCONCLUSIVE`; recovery failure ⇒ `FAILED` |
| `method` | ordered phases, each an ordered `steps:` list | first failure stops the timeline ⇒ `FAILED` |
| `verify` | once, at the end (**all** steps run even after failures, cumulative) | any non-finding failure ⇒ `FAILED` |
| `teardown` | **always**, even after ERRORED/FAILED; skipped under `--keep-up` / `KEEP_UP=1` | recorded, never changes the verdict |

A **phase** is:

```yaml
method:
  - phase: "SIGKILL worker-a; a peer recovers the job"
    steps:
      - run: docker.kill
        with: worker-a
```

There are no do/check/after buckets inside a phase; kind comes from the
verb, and steps interleave acting and observing freely.

## Tags

`tags:` is a flat list of plain strings. They carry no execution semantics;
they exist so `run` and `list` can select scenarios with `--include-tags` /
`--exclude-tags` boolean expressions (see the [CLI reference](/reference/cli/)).
Each tag must match `[A-Za-z0-9_./-]+` so it stays usable inside an expression;
`validate` flags anything else (rule 14).

## Teardown default

When the `teardown:` key is **absent**, Shinari runs `<lifecycle>.down`
(the one configured provider implementing `up`/`down`). When the key is
**present** (even empty), it replaces that default entirely.

## steadyState contract

Must be idempotent: it runs twice. An *active* probe (generate load, assert
it succeeds) must sample a fresh batch each run. `validate` warns when a
steadyState step resolves to a mutating action (rule 9).

## Interpolation

Each `${...}` is a **jq expression** evaluated over the scope, the same jq used
in `read:`/`capture:`, so there is one expression language. Every reference is
**namespaced**: the jq input is an object with four engine-owned namespaces, and
the first path segment names which one to read from.

| namespace | holds | bound by |
|---|---|---|
| `.vars.NAME` | a declared variable | a `vars:` block (project or scenario) |
| `.outputs.NAME` | a step result or capture | `as: NAME` or `capture: { NAME: ... }` |
| `.env.NAME` | a declared environment variable | the project's `env:` block (see [Project & discovery](/reference/project/)) |
| `.params.NAME` | a composed-verb parameter | a `params:` list, only inside a `kind: Provider` body |

So `${.vars.job}` reads a var, `${.outputs.rsp.value.total}` reaches into a
captured object, and full jq is available past the first two segments
(`${.outputs.total.value // 0}`, `${.outputs.runs.value | length}`). A reference
that is the entire value (`with: ${.outputs.job}`) preserves the result's type;
embedded references stringify (maps and lists as JSON), and a jq result of null
renders as empty.
Captures are scenario-global, ordered, last-write-wins, visible across sections.
A reference to a name that no namespace declares is a `validate` error (rule 10).

To pass a **literal** `${...}` through untouched (a shell snippet in `exec.run`,
a template payload), escape it with a second dollar: `$${HOME}` renders as
`${HOME}` and is never evaluated.

A step's result is captured as an **Observation envelope** `{value, output,
meta}`: `as: rsp` binds the whole envelope, so the payload is
`${.outputs.rsp.value}` and the call's facts are
`${.outputs.rsp.meta.durationMs}` / `${.outputs.rsp.meta.status}`. `read:` and
`capture:` operate on the payload. A per-step `timeout:` (seconds) fails the step
if the verb runs longer.

## Timeouts

A per-step `timeout:` (seconds) bounds one step: the step FAILs and is marked
timed-out if the verb runs longer, and its context is cancelled. A top-level
`timeout:` bounds the whole timeline (setup through verify): if it expires the
scenario is `FAILED` with `scenario exceeded timeout <N>s`. Teardown still runs
after a scenario timeout. The `http` provider honors whichever deadline applies
for any duration; with no step or scenario deadline it falls back to a 30s
per-request default.
