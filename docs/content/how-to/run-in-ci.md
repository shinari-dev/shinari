---
title: Run in CI
description: Wire exit codes, JUnit XML, and the findings report into a pipeline that stays meaningful.
weight: 40
---

**Goal:** a CI job that goes red only on *real change*: a regression, or a
finding that started passing.

## The job

```yaml
# .github/workflows/resilience.yml (excerpt)
- name: Build shinari
  run: go build -o shinari ./cli

- name: Validate (fail fast, no run)
  run: ./shinari -p tests/resilience validate

- name: Run the suite
  run: ./shinari -p tests/resilience --out reports run

- name: Publish JUnit results
  if: always()
  uses: mikepenz/action-junit-report@v4
  with:
    report_paths: reports/junit.xml

- name: Attach the findings ledger
  if: always()
  uses: actions/upload-artifact@v4
  with:
    name: findings
    path: reports/findings.md
```

## Interpret the exit code

| exit | verdict | CI meaning |
|---|---|---|
| 0 | `PASSED` | green: findings still failing as expected |
| 1 | `FAILED` | red: regression, **or a finding now passes (promote it)** |
| 2 | `ERRORED` | red: harness/infra problem, not a product problem |
| 3 | `INCONCLUSIVE` | red: system was never healthy; fix the baseline |
| 64 | usage | the pipeline invoked shinari wrong |

Route alerts accordingly: `1` pages the product team, `2`/`3` page whoever
owns the test environment. That separation is the reason the codes exist.

## Findings in JUnit output

A held finding renders as a **passed** testcase with the narrative in
`system-out`: CI dashboards stay green while the gap remains visible in the
test detail. The `results.json` artifact carries the full structure
(per-check verdicts, findings, injected faults, timings) for custom tooling.

## One run at a time

`shinari run` takes a per-project `flock`; a second concurrent run on the
same runner fails fast with exit 2 instead of corrupting the stack. If your
CI retries jobs, that's the guard that keeps retries honest.
