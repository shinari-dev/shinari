---
title: Getting started
description: Build the binary, run the quickstart project, and read your first findings report, in five minutes.
weight: 10
---

You assume your system recovers when a worker dies mid-task. In five minutes,
Shinari will make you prove it. By the end of this tutorial you will have built
the CLI, broken a running system on purpose, and turned "it should recover" into
a test with a verdict.

## 1. Build the binary

Shinari is a single static Go binary. From the repository root:

```sh
go build -o shinari ./cli
./shinari
```

You should see the command list: `init`, `validate`, `list`, `run`.

## 2. Meet the quickstart project

The repo ships a complete example under `examples/quickstart/`: a toy job
store driven entirely through shell, zero infrastructure required. Look at
its shape:

```text
examples/quickstart/
  project.yml                    # kind: Project (the root)
  shinari.lock.yml               # pinned providers
  providers/jobstore.yml         # kind: Provider (a composed provider)
  scripts/jobstore.sh            # the "system under test"
  scenarios/core/clean-complete.yml
  scenarios/recovery/worker-killed.yml
```

Nothing about this layout is mandatory. Shinari recognizes its files by
their `apiVersion`/`kind` header, not by name or location.

## 3. Validate before running

```sh
./shinari -p examples/quickstart validate
```

```text
[warn] rule 8: ... no lifecycle provider (up/down) configured ...
valid — 2 scenario(s), 2 warning(s)
```

`validate` is static: it resolves every verb, checks every argument and every
`${...}` reference, and never touches the system. The warning is expected;
this project uses no docker stack.

## 4. List what was discovered

```sh
./shinari -p examples/quickstart list
```

```text
core
  clean-complete — A job submitted and completed normally runs exactly once.
recovery
  worker-killed-mid-task — A worker dies mid-task and a peer recovers the job. ...
```

Scenarios group into **suites** by directory convention (`scenarios/<suite>/`).

## 5. Run the suite

```sh
./shinari -p examples/quickstart run
```

Each scenario prints its lifecycle phase by phase, then a verdict. Here is the
one that carries a finding:

```text
━━ worker-killed-mid-task ──────────────────────────────────
  setup     ✓ jobstore.reset
  steady    ✓ jobstore.healthy
  method    ✓ jobstore.submit
            ✓ wait_until
            ✓ jobstore.crash_worker
            ✓ jobstore.recover
  recovery  ✓ jobstore.healthy
  verify    ✓ wait_until
            ✓ jobstore.status
            ✓ job completed after the crash
            ✓ jobstore.runs
            ◆ no duplicate run (exactly once) · FINDING: recovery re-runs the whole job: duplicate work until idempotent replay ships; operators dedupe downstream today
  teardown  ✓ jobstore.reset

  ✔ PASSED · 1 finding held · 21ms

5 scenarios: 5 passed — 1 finding held (0s)
reports → shinari-out/{results.tsv,results.json,junit.xml,journal.jsonl,findings.md,findings.sarif}
```

The ◆ line is the point: the exactly-once assertion **failed**, and the run is
still **green** (exit `0`). That failure is a *known, documented gap*, declared
with `finding:` in the scenario, so the suite stays a signal instead of a wall
of ignored red. A step that injects a fault in `method` is flagged with a `⚡
(fault injected)` line; here the crash verb declares no effect, so it renders as
a plain step.

## 6. Read the reports

Every run writes reports under `shinari-out/` (override with `--out`):

```sh
cat shinari-out/findings.md
```

Per scenario you get **Injected** (which faults ran), **Held** (which
assertions passed), and **Gapped** (the findings, with the observed failure).
There is also `results.json`, `junit.xml`, `results.tsv`, `journal.jsonl` (the
full event stream), and `findings.sarif` for code-scanning tools.

## Where you are

You ran a suite, broke a system on purpose, and read its ledger. Next:
[write your own scenario](/tutorials/first-scenario/).
