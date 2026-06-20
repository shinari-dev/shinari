---
title: Your first scenario
description: Write a Project and a Scenario from an empty directory and watch each section of the timeline do its job.
weight: 20
---

You will build a tiny project from scratch (a Project resource, one Scenario)
and learn the five sections of the timeline by triggering each one.

## 1. Create the project root

Make a directory and a `project.yml`:

```yaml
apiVersion: shinari/v1
kind: Project
name: my-first-project

vars:
  greeting: hello

providers:
  exec: {}
```

Three lines of header make a file a Shinari **resource**. The `exec` provider
is the escape hatch: it gives you `exec.run` so you can test the harness with
nothing but shell.

## 2. Write a scenario

Any filename works; the header is what counts. Create `survive.yml`:

```yaml
apiVersion: shinari/v1
kind: Scenario
name: survive-nothing
description: The smallest possible timeline.

setup:
  - run: exec.run
    with: "echo preparing"

method:
  - phase: "Do the thing"
    steps:
      - run: exec.run
        with: "echo ${.vars.greeting}"
        as: said

verify:
  - run: assert
    with:
      of: "${.outputs.said.value}"
      equals: hello
    desc: "we said hello"

teardown:
  - run: exec.run
    with: "echo cleaning up"
```

Read it top to bottom; it is the test lifecycle:

| section | role | on failure |
|---|---|---|
| `setup` | arrange, once | run is `ERRORED`: the harness never came up |
| `method` | ordered phases of faults + observations | run is `FAILED` |
| `verify` | cumulative, terminal checks | run is `FAILED` |
| `teardown` | always runs, even after failure | never changes the verdict |

## 3. Validate, then run

```sh
shinari validate     # from inside the directory; cwd is the default project
shinari run
```

```text
=== survive-nothing
  ✓ exec.run
  -- Do the thing
  ⚡ fault injected: exec.run
  ✓ exec.run
  ✓ we said hello
  ✓ exec.run
  => PASSED
```

Note the capture: `as: said` stored the step's output under the `outputs`
namespace, and `${.outputs.said.value}` read it back in `verify`. The var read
earlier (`${.vars.greeting}`) lives in its own namespace; every `${...}`
reference names the namespace it resolves against. Captures are scenario-global,
ordered, last-write-wins.

## 4. Break it, on purpose

Change the assertion to `equals: goodbye` and run again:

```text
  ✗ we said hello — assert failed: expected hello == goodbye
  => FAILED
```

Exit code `1`: *your system broke*. Now break the harness instead; change
`setup` to `exec.run, with: "exit 1"`:

```text
  => ERRORED
```

Exit code `2`: *the run never happened*. Distinct exit codes let CI tell the
difference without parsing logs.

## 5. Add a steady-state gate

Insert between `setup` and `method`:

```yaml
steadyState:
  - run: exec.run
    with: "test -t 0 || true"
    kind: assertion
```

`steadyState` runs **before** `method` as a gate: if the system isn't
healthy before you inject anything, the run is `INCONCLUSIVE` (exit `3`),
because a fault test against a broken system proves nothing. It then re-runs
**after** `method` as the recovery check.

## Where you are

You know the timeline, captures, and the verdict matrix. Next, the
differentiator: [track your first finding](/tutorials/first-finding/).
