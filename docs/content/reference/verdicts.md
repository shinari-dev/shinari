---
title: Verdicts
description: Check-level and scenario-level outcomes, their resolution order, and the exit-code mapping.
weight: 60
---

## Check level

| verdict | meaning |
|---|---|
| `PASS` | the check held |
| `FAIL` | the check did not hold (or errored) |
| `SKIP` | tri-state skip via `onAbsent: skip`, or an action under `-dry-run` |
| `FINDING` | a `finding:`-marked check failed **as expected**: counted separately, keeps the run green |

## Scenario level

| verdict | when | exit |
|---|---|---|
| `PASSED` | every check PASS/SKIP; findings still fail as expected | 0 |
| `FAILED` | a non-finding check FAILs anywhere after setup, or a `finding:` unexpectedly PASSes | 1 |
| `ERRORED` | a `setup` step failed: the harness could not be established; the run never happened | 2 |
| `INCONCLUSIVE` | `steadyState` failed **before** `method`: never healthy, the run proves nothing | 3 |

These map onto familiar test-runner verdicts (JUnit
passed/failure/skipped/error; INCONCLUSIVE is NUnit's). Only FINDING is novel.

## Resolution order

```text
setup fails                       ⇒ ERRORED      (teardown still runs)
steadyState fails (gate, before)  ⇒ INCONCLUSIVE (teardown still runs)
method phase fails                ⇒ FAILED       (skip to teardown)
steadyState fails (recovery)      ⇒ FAILED       (verify still runs, cumulative)
any verify non-finding FAIL       ⇒ FAILED
any finding: that PASSes          ⇒ FAILED ("promote this to a hard assertion")
otherwise                         ⇒ PASSED
teardown                          always runs, never changes the verdict
```

## Run level

One `shinari run` may execute many scenarios; the process exit code is the
**worst** scenario verdict, ranked `ERRORED > FAILED > INCONCLUSIVE > PASSED`.
The full per-scenario detail is always in `results.json`.
