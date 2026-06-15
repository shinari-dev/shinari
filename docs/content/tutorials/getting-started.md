---
title: Getting started
description: Build the binary, run the quickstart project, and read your first findings report, in five minutes.
weight: 10
---

By the end of this tutorial you will have built Shinari, executed a resilience
suite against a toy system, and seen the harness hold a **finding** green.

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
./shinari -C examples/quickstart validate
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
./shinari -C examples/quickstart list
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
./shinari -C examples/quickstart run
```

Watch the stream. Three glyphs matter:

```text
  ⚡ fault injected: jobstore.crash_worker
  ▷ gate observed: RUNNING
  ◆ FINDING no duplicate run (exactly once)
  => PASSED

2 scenario(s): 2 passed, 0 failed, 0 errored, 0 inconclusive — 1 finding(s) held
```

The exactly-once assertion **failed**, and the run is **green** (exit `0`).
That is the point: the failure is a *known, documented gap*, declared with
`finding:` in the scenario, so the suite stays a signal instead of a wall of
ignored red.

## 6. Read the reports

Every run writes reports under `shinari-out/` (override with `-out`):

```sh
cat shinari-out/findings.md
```

Per scenario you get **Injected** (which faults ran), **Held** (which
assertions passed), and **Gapped** (the findings, with the observed failure).
There is also `results.json`, `junit.xml`, `results.tsv`, and
`journal.jsonl`: the full event stream.

## Where you are

You ran a suite, broke a system on purpose, and read its ledger. Next:
[write your own scenario](/tutorials/first-scenario/).
