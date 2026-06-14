---
title: Step envelope
description: The one step shape — every reserved key, exactly what it does, in evaluation order.
weight: 30
---

Every step everywhere (sections, phases, composed-provider bodies) is the
same shape. The verb is a **value**, never a key:

```yaml
- run: <provider>.<verb>        # required — or an unprefixed language builtin
  with: <scalar | list | map>   # args, validated against the verb's arg spec
  as: <name>                    # capture the whole result value
  read: <jq expr>               # transform the result value first
  capture: { <name>: <jq expr>, ... }   # named captures from the result
  desc: <text>                  # narrative; used in reports and failure messages
  onAbsent: skip                # tri-state SKIP instead of failure
  skipReason: <text>            # rendered when skipped
  finding: <text>               # mark an expected failure (assertion-kind checks only)
  kind: action|probe|assertion  # override — for usage-dependent verbs (exec.run)
  effect: outage|degradation    # declare a fault when injecting it via a polymorphic verb
```

Unknown keys are a **parse error**, not ignored.

## Key semantics

| key | detail |
|---|---|
| `run` | resolved against the configured providers' verb specs, or the builtin table when unprefixed. Unresolvable ⇒ failure (or SKIP under `onAbsent: skip`) |
| `with` | a scalar or list binds to the verb's *primary* arg (`with: worker-a` ≡ `with: { service: worker-a }`); a map is checked key-by-key: unknown and missing-required args are errors |
| `read` | jq over the result value, applied **before** `as:`/`capture:`. Real jq: `.state`, `.items \| length` |
| `as` | binds the (post-`read`) value into the scenario-global capture scope |
| `capture` | each entry binds `name := jq(expr, value)` — for plucking several fields at once |
| `onAbsent: skip` | when resolution or interpolation fails, the check becomes `SKIP` with `skipReason` instead of `FAIL` |
| `finding` | inverts the contract: failure ⇒ `FINDING` (green), success ⇒ `FAILED` with "promote this". Allowed only on assertion-kind checks (validate rule 5) |
| `kind` | overrides the verb's declared kind for this step. In practice: `exec.run` defaults to `action`; mark a read-only script `kind: probe` so dry-run and steadyState treat it correctly |
| `effect` | declares the fault this step injects (`outage` drops/blocks work, `degradation` slows it). Native fault verbs already carry it; set it on a step when the fault rides a polymorphic verb — `exec.run` running `tc`/`iptables`, or `http.post` to a chaos endpoint — so fault tracking and validate's recovery rule see it |

## Evaluation order

```text
resolve run → interpolate with → bind args → execute verb
            → read → as / capture → verdict (finding logic last)
```

## Flow style

YAML flow mappings (`- { run: docker.kill, with: worker-a }`) parse
identically and are accepted everywhere — but block style is the documented
convention: it stays readable once `with:` nests.

```yaml
- run: docker.kill
  with: worker-a
- run: app.submit
  with:
    job: sleep
  as: job
```
