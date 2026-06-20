---
title: Validate rules
description: The twelve static checks, what triggers each, and which are errors versus warnings.
weight: 70
---

`shinari validate` runs before anything touches a system. Every finding names
file, scenario, step, and reason.

| # | rule | severity |
|---|---|---|
| 1 | **Header**: every resource has a recognized `apiVersion` + `kind`; a *marked but malformed* file is an error, never a silent skip | error |
| 2 | **Schema**: each step is one `run:` with only reserved envelope keys; `with:` matches the verb's arg spec (unknown arg, missing required, missing/duplicate assert operator) | error |
| 3 | **Provider resolution**: every `<provider>.<verb>` resolves to a configured instance and declared verb (suppressed by `onAbsent: skip`) | error |
| 4 | **Macro nesting ≤ 1**: composed verbs call native verbs/builtins freely, another composed verb only one level deep | error |
| 5 | **`finding:` placement**: only on checks whose effective kind is `assertion` | error |
| 6 | **Capture-before-settle**: no reference to a `background` capture before its `stop_background` | error |
| 7 | **Recovery invariant**: a recovery-shaped scenario (an outage-class fault in method + captured work + verify awaiting it) must assert exactly-once (`equals: 1`) or carry a `finding:`. "Outage-class" is declared by the verb's `Effect`, so third-party faults count too | error when fully shaped, warn on partial match |
| 8 | **One lifecycle provider**: at most one configured provider implements `up`/`down`; several is an error, zero a warning (pure exec/http suites are legitimate) | error / warn |
| 9 | **steadyState idempotency**: warn when steadyState contains a mutating action, it re-runs after method | warn |
| 10 | **Interpolation closure**: every `${...}` is namespaced and resolves to a declared name: `${.vars.X}` to a var, `${.outputs.X}` to an *earlier* capture (in execution order), `${.env.X}` to a key in the project's `env:` allowlist, `${.params.X}` to a composed-verb param (composed-provider bodies are checked against their `params:`). An undeclared name, or an unnamespaced reference, is an error | error |
| 11 | **Degradation observed**: warn when a `degradation` fault is injected in `method` but nothing observes its effect (no `sample`, no `${.outputs....meta.durationMs}` assertion) | warn |
| 12 | **Parallel branches**: a `parallel` step has a non-empty `branches:` list with no empty branch, and no branch references a capture bound only in a *sibling* branch (concurrent branches have no ordering, so reference it after the block) | error |

## Branch steps

Steps inside a `parallel` branch are validated **exactly like top-level steps**,
recursively (including nested `parallel`). Rules 2, 5, 6, 9, and 10 all apply
within a branch. References resolve against a branch-local scope: the `vars` and
`env` namespaces, `outputs` bound before the block, and that branch's own earlier
`outputs`; a sibling branch's outputs are out of scope (rule 12 above), and
outputs a branch binds become visible to steps after the block.

## Exit behaviour

`validate` exits `1` if any **error**-severity finding exists, `0` when the
project is clean or carries only warnings.

## Reading a finding

```text
[error] rule 3: scenarios/net/s1.yml scenario partition-db step toxiproxi.partition:
  verb "toxiproxi.partition": no provider instance named "toxiproxi" is configured
```

Severity, rule number, file, scenario, step, reason, in that order, always.
