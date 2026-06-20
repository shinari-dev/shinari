---
name: writing-resilience-scenario
description: Use when authoring or editing a Shinari resilience test scenario (a kind:Scenario YAML), adding a fault-injection or recovery test, wiring providers/builtins/assertions, or recording a known gap as a finding. Covers the scenario lifecycle, step shape, result envelope, and the validate loop.
---

# Writing a Shinari resilience scenario

## Overview

A scenario is a `kind: Scenario` YAML file. The engine brings a real system up,
gates on a healthy baseline, injects controlled deterministic faults, asserts
how the system survives, then tears down. A scenario is data, not narrative:
every step names a verb and the engine runs them in a fixed lifecycle.

**Core loop: write a little, then run `validate`.** The validator is a static
judge whose rules catch the mistakes that are easy to make and hard
to spot (recovery contract, idempotency, reference order, fault observed).
Treat a clean `validate` as the definition of "well-formed", not your reading.

```sh
./shinari -p <project-dir> validate    # static checks, no infra
./shinari -p <project-dir> list        # scenarios grouped by suite
```

## Workflow

1. **Find the project root** (the dir holding `project.yml`) and which provider
   instances it declares. The `run:` prefix of every step is an *instance name*
   from that file (e.g. `http.get` needs an `http:` instance). Do not invent
   instance names; read `project.yml`.
2. **Pick the suite/file**: `scenarios/<suite>/<name>.yml` is convention only.
3. **Write the header, then fill the lifecycle sections** (below).
4. **Run `validate`. Fix every error and read every warning.** Iterate.
5. Do not add SPDX headers to scenario YAML; those are for Go source only.

## The lifecycle (sections run in this order)

| Section | Holds | Notes |
|---|---|---|
| `setup` | one-shot actions to bring the system up | `docker.up`, resets |
| `steadyState` | **probes/assertions only** | gate: if it fails before method, verdict is INCONCLUSIVE. **Re-runs automatically after method** to confirm recovery, so it must be idempotent (rule 9: no one-shot mutating action here) |
| `method` | a list of **phases**, each `{phase: name, steps: [...]}` | where faults are injected; this is the only section with the phase wrapper |
| `verify` | assertions over what was captured | the verdict |
| `teardown` | cleanup, always runs | absent teardown defaults to `[docker.down]` |

A scenario needs **exactly one lifecycle provider** (something with `up`/`down`,
i.e. `docker`). Zero is a warning (a pure http/exec suite is legitimate); two or
more is an error (rule 8).

## Step shape

A step is a mapping with a `run:` and a closed set of envelope keys. Any other
top-level key is a parse error.

```yaml
- run: <instance>.<verb>   # or an unprefixed builtin (assert, sleep, ...)
  with: <scalar | list | map>   # verb args; scalar/list bind the verb's primary arg
  as: <name>                    # capture the whole result under ${.outputs.<name>}
  read: <jq>                    # transform the result value before as:/capture:
  capture: { id: <jq> }         # bind extracted fields
  desc: <string>                # human label
  when: <jq predicate>          # value-gated SKIP: run only if the predicate is truthy
  kind: <action|probe|assertion>  # override the verb's kind (the exec.run escape)
  effect: <outage|degradation>     # declare a fault a polymorphic verb injects
  finding: <string>             # mark this assertion a known, expected gap
  timeout: <seconds>
  onAbsent: skip                # skip if the verb is not configured
```

Reserved envelope keys (the only ones allowed): `run, with, as, read, capture,
desc, when, onAbsent, skipReason, finding, kind, effect, timeout`. Note `finding:`
is a **step key**, not a `with:` key.

**`when:`** is a jq predicate over the scope (`when: "${.outputs.n > 1}"`),
evaluated before the verb runs; falsey ⇒ the step is `SKIP`. It is a *guard, not
a branch* — there is no `then`/`else` and no nested body. jq truthiness applies
(only `false`/`null` are falsey). A `when:`-guarded exactly-once assertion does
**not** satisfy the recovery contract (rule 7 still fires).

### Interpolation, namespaces, and the result envelope

Every string is interpolated with `${...}` jq expressions, and **every reference
is namespaced** into one of four engine-owned scopes:

| Namespace | Holds | Bound by |
|---|---|---|
| `.vars.NAME` | a declared variable | a `vars:` block (scenario or project) |
| `.outputs.NAME` | a step result or capture | `as: NAME` or `capture: { NAME: ... }` |
| `.env.NAME` | an injected environment value | the project's `env:` block (see below) |
| `.params.NAME` | a composed-provider parameter | a `params:` list, only inside `kind: Provider` bodies |

There are no bare references: a var declared as `job` is `${.vars.job}`, a capture
bound by `as: r` is `${.outputs.r...}`.

A captured result is an **envelope**, not a bare value. Address it through its
fields (under `.outputs.<name>`):

| Path | Is | Example |
|---|---|---|
| `${.outputs.r.value}` | structured result (decoded JSON, rows, stats) | `${.outputs.state.value}`, `${.outputs.load.value.p99}` |
| `${.outputs.r.output}` | raw text output | logs/diagnostics |
| `${.outputs.r.meta.status}` | HTTP status code | `${.outputs.r.meta.status}` |
| `${.outputs.r.meta.durationMs}` | call latency, on every result | latency asserts |

There is **no top-level `.status`, and no `.error` field.** A failed call fails
the step; you do not test for an error field. After `as: r`, use
`${.outputs.r.value...}` or `${.outputs.r.meta...}`.

In `read:`/`capture:` (and `wait_until`'s `read:`) the jq input `.` is the
**value**, and the envelope's other channels are bound as jq variables: `$meta`
(`$meta.status`, `$meta.bytes`, `$meta.durationMs`) and `$output` (raw text).
This lets a probe gate directly on a status code without binding the whole
envelope, e.g. `read: "$meta.status"` with `in: [200, 401, 403]`.

### Environment injection (`env:`)

A **project** (`kind: Project`) may declare an `env:` block, shaped like `vars:`.
Each key's value is a **default**; the matching process environment variable
overrides it. A key with a **null** value (no default) is **required**: the run
fails with exit code 2 if it is unset.

```yaml
env:
  DATABASE_URL:        # required (null default); run errors (exit 2) if unset
  PORT: 8080           # default 8080, overridden by $PORT when set
```

Reference declared env values as `${.env.DATABASE_URL}`. The `env:` block is an
**allowlist**: referencing an env name not declared there is a validate error.
Env is project-level only; there is no scenario-level `env:` block. The CLI
resolves the block (core never reads the process environment).

## Two semantics that drive the design

**Findings ledger.** A step with `finding:` marks an assertion as a *known,
expected* gap. When it fails it is recorded as `FINDING` and the scenario stays
**green**. When it starts *passing* (the gap was fixed) the run flips to
`FAILED` ("promote this to a hard assertion"). `finding:` is only legal on
assertion-kind checks (rule 5).

**Recovery contract (rule 7).** If method injects an outage fault **and**
captures an id/work item **and** verify awaits that work, the scenario is
"recovery-shaped" and *must* either assert exactly-once (`count` `equals: 1`) or
carry a `finding:` on the relevant assertion. This is the single most common
validate error when writing recovery tests: a worker dies and a peer recovers
the job, so you must say whether the job ran exactly once, or record the
duplicate-work gap as a finding. See the `worker-killed` example.

## Idioms

- **Gate faults on observed state, not a bare timer.** Inject mid-load by
  running `wait_until` (probe healthy) then the fault inside one `parallel`
  branch while another branch drives `load.run`. Use `sleep` only for a real
  physical settle (e.g. waiting for a sidecar to attach).
- **Observe degradation.** A `degradation` fault that nothing measures is a
  warning (rule 11). Assert on `${.outputs....meta.durationMs}` or use `sample`/`load`
  percentiles.
- **References resolve in execution order** (rules 6, 10, 12). A capture is
  visible only to later steps; a capture bound only in a sibling `parallel`
  branch is not visible to its siblings; a `background` capture settles only at
  `stop_background`.

## Reference and template

- **[reference.md](reference.md)** — the full catalog: every native provider
  (exec, http, docker, toxiproxy, net, sql, prom, load) with config and verbs,
  every builtin (assert, sleep, wait_until, background, stop_background, sample,
  parallel, repeat), the assert operators, composed providers, and all fourteen
  validation rules. Read it instead of reverse-engineering verb signatures from
  Go source.
- **[template.yml](template.yml)** — an annotated, validate-clean scaffold to
  copy.

## Common mistakes

| Mistake | Fix |
|---|---|
| `${.outputs.r.status}` / `${.outputs.r.error}` | use `${.outputs.r.meta.status}`; there is no error field |
| Bare/un-namespaced ref (`${.r}`, `${.job}`) | namespace it: `${.outputs.r.value}`, `${.vars.job}` |
| Recovery test fails rule 7 | assert exactly-once or add a `finding:` |
| One-shot mutating verb in `steadyState` | move it to `setup`; steadyState re-runs |
| `finding:` under `with:` | `finding:` is a step-level key |
| Invented instance name in `run:` | the prefix must match a `project.yml` instance |
| Phase steps without the `{phase:, steps:}` wrapper | only `method` uses phases |
| SPDX header on the scenario | scenarios carry none |
| Degradation fault, nothing measured | add a latency assert or `sample` |
| Shipping without running `validate` | always validate before claiming done |
