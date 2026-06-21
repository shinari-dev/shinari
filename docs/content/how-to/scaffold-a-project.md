---
title: Start a new project
description: Go from an empty directory to a passing resilience scenario in one command with shinari new.
weight: 5
---

**Goal:** a working Shinari project, validated and green, without copying an
example by hand.

## One command

```sh
shinari new my-service
```

This writes a complete, runnable project into `my-service/`:

```text
my-service/
  project.yml                         the project: vars and configured providers
  .gitignore                          ignores shinari-out/, .jobstore/, the lock file
  README.md                           what each file does and how to run it
  providers/jobstore.yml              a composed provider over exec, zero Go
  scripts/jobstore.sh                 a toy job store the provider drives
  scenarios/core/clean-complete.yml   the happy path: a job runs exactly once
  scenarios/recovery/worker-killed.yml a worker dies mid-task; recovery is gapped
```

The project name comes from the directory's basename. Everything runs through
`exec` and a shell script, so there is no infrastructure to stand up.

## Run it

```sh
shinari -p my-service validate
shinari -p my-service run
```

Both are green on the first try. `run` ends `PASSED` with one finding held: the
recovery scenario asserts that a recovered job runs exactly once, the toy store
re-runs it, and that known gap is recorded with `finding:` so the run stays
green until the gap is fixed. This is the [findings ledger](/concepts/findings-ledger/)
in miniature.

## Make it yours

The scaffold is a starting point, not a fixed shape. Point `providers/jobstore.yml`
at your own system (or add the native [`docker`](/how-to/lifecycle-and-stacks/),
[`http`](/reference/providers/), and [`toxiproxy`](/how-to/inject-network-faults/)
providers to `project.yml`), rewrite the example scenarios against your own
vocabulary, and drop new scenarios under `scenarios/`. Discovery walks the tree,
so the layout is a convention you are free to change.

`new` never overwrites. If the target already holds a `project.yml`, or any file
it would write already exists, it writes nothing and exits `64`.
