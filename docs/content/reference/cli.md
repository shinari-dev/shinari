---
title: CLI
description: Commands, flags, environment variables, and exit codes of the shinari binary.
weight: 10
---

```text
shinari [flags] <command> [target...]
```

A **target** is a scenario name or a suite name, resolved by discovery — never
a file path. No targets means all scenarios.

## Commands

| command | effect |
|---|---|
| `init` | resolve every configured provider; write `shinari.lock.yml` (builtin versions, local-provider checksums) |
| `validate` | run the [static rules](/reference/validate/); no execution. Exit 1 on errors, 0 on warnings only |
| `list` | print discovered scenarios grouped by suite |
| `run` | execute targeted scenarios; write reports; exit by verdict |

## Flags

| flag | default | meaning |
|---|---|---|
| `-C <dir>` | `.` | project directory (the discovery root) |
| `-out <dir>` | `shinari-out` | report directory for `run` |
| `-dry-run` | off | skip all *action* steps; probes and assertions still run |

## Environment

| variable | effect |
|---|---|
| `KEEP_UP=1` | skip the entire `teardown` section — preserve the stack for inspection |

## Exit codes

| code | meaning |
|---|---|
| `0` | `PASSED` — all checks pass/skip; findings still fail as expected |
| `1` | `FAILED` — a check regressed, or a `finding:` unexpectedly passes |
| `2` | `ERRORED` — setup failed; the harness could not be established (also: report I/O failure, concurrent-run lock held) |
| `3` | `INCONCLUSIVE` — steadyState failed before method |
| `64` | usage error (unknown command/target, bad flags) |

With several scenarios in one run, the **worst** verdict wins, ranked
`ERRORED > FAILED > INCONCLUSIVE > PASSED`.

## Concurrency guard

`run` takes an exclusive `flock` keyed by the absolute project path
(`$TMPDIR/shinari-<hash>.lock`). A second simultaneous run against the same
project exits 2 immediately.

## Report files

`run` writes five renderings of the same result into `-out`:

| file | content |
|---|---|
| `results.json` | full structured result: per-check verdicts, findings, timings, injected/held/gapped, roll-up verdict + exit code |
| `junit.xml` | one `<testsuite>` per scenario; findings render as passes with a `system-out` note |
| `results.tsv` | one row per check: scenario, section, check, verdict, duration, error |
| `journal.jsonl` | the serialized event stream, one event per line |
| `findings.md` | the human ledger: injected / held / gapped per scenario |
