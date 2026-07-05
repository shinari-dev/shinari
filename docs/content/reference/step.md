---
title: Step envelope
description: The one step shape, every reserved key, exactly what it does, in evaluation order.
weight: 30
---

Every step everywhere (sections, phases, composed-provider bodies) is the
same shape. The verb is a **value**, never a key:

```yaml
- run: <provider>.<verb>        # required (or an unprefixed language builtin)
  with: <scalar | list | map>   # args, validated against the verb's arg spec
  as: <name>                    # capture the whole result value
  read: <jq expr>               # transform the result value first
  capture: { <name>: <jq expr>, ... }   # named captures from the result
  desc: <text>                  # narrative; used in reports and failure messages
  when: <jq predicate>          # value-gated SKIP: falsey skips the step
  onAbsent: skip                # tri-state SKIP instead of failure
  skipReason: <text>            # rendered when skipped
  finding: <text>               # mark an expected failure (assertion-kind checks only)
  id: <slug>                    # pin the finding's ledger identity (validate rule 15)
  kind: action|probe|assertion  # override, for usage-dependent verbs (exec.run)
  effect: outage|degradation    # declare a fault when injecting it via a polymorphic verb
  timeout: <seconds>            # per-step deadline; the verb's context is cancelled
```

Unknown keys are a **parse error**, not ignored.

## Key semantics

| key | detail |
|---|---|
| `run` | resolved against the configured providers' verb specs, or the builtin table when unprefixed. Unresolvable ⇒ failure (or SKIP under `onAbsent: skip`) |
| `with` | a scalar or list binds to the verb's *primary* arg (`with: worker-a` ≡ `with: { service: worker-a }`); a map is checked key-by-key: unknown and missing-required args are errors |
| `read` | jq over the result value, applied **before** `as:`/`capture:`. The result's other side-channels are bound as jq variables `$meta` and `$output`, so a transform can reach a status code that is not in the value: `$meta.status`. Real jq: `.state`, `.items \| length` |
| `as` | binds the result as an Observation envelope `{value, output, meta}` (the post-`read` payload under `.value`, plus `meta.durationMs` and provider facts) into the scenario-global capture scope. Read it with jq: `${.outputs.name.value}`, `${.outputs.name.meta.durationMs}` |
| `capture` | each entry binds `name := jq(expr, value)` over the payload, with `$meta`/`$output` also in scope (`code: "$meta.status"`), for plucking several fields at once |
| `when` | a jq predicate over the scope (`when: "${.outputs.n > 1}"`); evaluated **first**, before resolution. Falsey ⇒ the step is `SKIP` (with `skipReason`). jq truthiness applies: only `false` and `null` are falsey. This is a *guard*, not a branch — there is no `then`/`else` and no nested body. A `when:`-guarded exactly-once assertion does not satisfy the recovery invariant (validate rule 7 still fires) |
| `onAbsent: skip` | when resolution or interpolation fails, the check becomes `SKIP` with `skipReason` instead of `FAIL` |
| `finding` | inverts the contract: failure ⇒ `FINDING` (green), success ⇒ `FAILED` with "promote this". Allowed only on assertion-kind checks (validate rule 5). A check that cannot even be evaluated (unresolvable verb, broken interpolation) fails hard instead of recording a finding |
| `id` | pins the finding's ledger identity; without it the identity derives from the check and changes when the check is edited (validate rule 15) |
| `kind` | overrides the verb's declared kind for this step. In practice: `exec.run` defaults to `action`; mark a read-only script `kind: probe` so dry-run and steadyState treat it correctly |
| `effect` | declares the fault this step injects (`outage` drops/blocks work, `degradation` slows it). Native fault verbs already carry it; set it on a step when the fault rides a polymorphic verb (`exec.run` running `tc`/`iptables`, or `http.post` to a chaos endpoint) so fault tracking and validate's recovery rule see it |
| `timeout` | optional per-step deadline in seconds; the step FAILs and is marked timed-out if the verb runs longer (the verb's context is cancelled) |

## Evaluation order

```text
when (skip if falsey) → resolve run → interpolate with → bind args → execute verb
                      → read → as / capture → verdict (finding logic last)
```

## Flow style

YAML flow mappings (`- { run: docker.kill, with: worker-a }`) parse
identically and are accepted everywhere, but block style is the documented
convention: it stays readable once `with:` nests.

```yaml
- run: docker.kill
  with: worker-a
- run: app.submit
  with:
    job: sleep
  as: job
```
