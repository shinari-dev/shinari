---
title: CLI
description: Commands, flags, environment variables, and exit codes of the shinari binary.
weight: 10
---

```text
shinari [--project <dir>] <command> [flags] [target...]
```

A **target** is a scenario name or a suite name, resolved by discovery, never
a file path. No targets means all scenarios.

The CLI is built on Cobra, so flags use GNU style (`--long`, with single-letter
shorthands like `-p`) and may appear before or after the command. Run
`shinari --help` or `shinari <command> --help` for generated usage, and
`shinari --version` for the version.

## Commands

| command | effect |
|---|---|
| `new <dir>` | scaffold a complete, runnable project into `<dir>` (see below) |
| `init` | resolve every configured provider; write `shinari.lock.yml` (builtin versions, local-provider checksums) |
| `validate` | run the [static rules](/reference/validate/); no execution. Exit 1 on errors, 0 on warnings only |
| `list` | print discovered scenarios grouped by suite |
| `run` | execute targeted scenarios; write reports; exit by verdict |

### new

```sh
shinari new my-service
```

`new <dir>` writes a complete, runnable project into `<dir>`: a `project.yml`, a
composed `jobstore` provider over `exec`, a shell-backed toy job store, a
`.gitignore`, a README, and two example scenarios (a happy path and a recovery
scenario that records a known gap as a finding). The project name is taken from
the directory's basename. Nothing in it needs infrastructure, so the next two
steps are green immediately:

```sh
shinari -p my-service validate
shinari -p my-service run
```

`new` never overwrites: if `<dir>` already holds a `project.yml`, or any file it
would write already exists, it writes nothing and exits `64`.

## Flags

Global (any command, any position):

| flag | default | meaning |
|---|---|---|
| `--project`, `-p <dir>` | `.` | project directory (the discovery root) |
| `--version` | | print the version and exit |

`run`:

| flag | default | meaning |
|---|---|---|
| `--out`, `-o <dir>` | `shinari-out` | report directory |
| `--dry-run` | off | skip all *action* steps; probes and assertions still run |
| `--include-tags <expr>` | | run only scenarios matching the tag expression |
| `--exclude-tags <expr>` | | drop scenarios matching the tag expression |

`list`:

| flag | default | meaning |
|---|---|---|
| `--include-tags <expr>` | | list only scenarios matching the tag expression |
| `--exclude-tags <expr>` | | drop scenarios matching the tag expression |

## Filtering by tag

A scenario may declare `tags:` (a flat list of strings). `run` and `list`
filter on them with JUnit5-style boolean expressions: `&` (and), `|` (or),
`!` (not), and parentheses. Repeating the include/exclude flags is not needed;
the strategy lives in the expression.

```sh
shinari run --include-tags 'slow & redis'        # both tags
shinari run --include-tags 'net | slow'          # either tag
shinari list --include-tags '(net | slow) & !flaky'
shinari run --exclude-tags flaky                 # everything except flaky
```

The selected set is `(matches --include-tags, or none given) AND NOT (matches
--exclude-tags)`; exclusion wins. A filter that matches nothing is not an error:
`run` prints `no scenarios matched` and exits 0. Tag filters compose with
positional targets by intersection.

## Environment

| variable | effect |
|---|---|
| `KEEP_UP=1` | skip the entire `teardown` section, preserving the stack for inspection |

## Exit codes

| code | meaning |
|---|---|
| `0` | `PASSED`: all checks pass/skip; findings still fail as expected |
| `1` | `FAILED`: a check regressed, or a `finding:` unexpectedly passes |
| `2` | `ERRORED`: setup failed; the harness could not be established (also: report I/O failure, concurrent-run lock held) |
| `3` | `INCONCLUSIVE`: steadyState failed before method |
| `64` | usage error (unknown command/target, bad flags) |

With several scenarios in one run, the **worst** verdict wins, ranked
`ERRORED > FAILED > INCONCLUSIVE > PASSED`.

## Concurrency guard

`run` takes an exclusive `flock` keyed by the absolute project path
(`$TMPDIR/shinari-<hash>.lock`). A second simultaneous run against the same
project exits 2 immediately.

## Report files

`run` writes five renderings of the same result into `--out`:

| file | content |
|---|---|
| `results.json` | full structured result: per-check verdicts, findings, timings, injected/held/gapped, roll-up verdict + exit code |
| `junit.xml` | one `<testsuite>` per scenario; findings render as passes with a `system-out` note |
| `results.tsv` | one row per check: scenario, section, check, verdict, duration, error |
| `journal.jsonl` | the serialized event stream, one event per line |
| `findings.md` | the human ledger: injected / held / gapped per scenario |
