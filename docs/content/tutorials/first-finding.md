---
title: Your first finding
description: Turn a known gap into an executable, self-maintaining contract — and watch CI flip when someone fixes it.
weight: 30
---

This is the heart of Shinari. You will declare a known failure as a
`finding:`, watch the suite stay green while the gap exists, then "fix" the
system and watch the suite go red — demanding promotion.

## 1. A system with a gap

Reuse the project from [Your first scenario](/tutorials/first-scenario/), and
fake a service that duplicates work on recovery. Create `dupes.yml`:

```yaml
apiVersion: shinari/v1
kind: Scenario
name: crash-recovery-duplicates
description: Recovery re-runs the whole job — a known duplicate-work gap.

setup:
  - run: exec.run
    with: "echo 0 > /tmp/shinari-tutorial-runs"

method:
  - phase: "Submit, crash, recover"
    steps:
      - run: exec.run               # the job runs
        with: "echo 1 > /tmp/shinari-tutorial-runs"
      - run: exec.run               # recovery re-runs it
        with: "echo 2 > /tmp/shinari-tutorial-runs"

verify:
  - run: exec.run
    with: "cat /tmp/shinari-tutorial-runs"
    as: runs
    kind: probe
  - run: assert
    with:
      of: "${.runs.value}"
      equals: 1
    desc: "exactly once"
    finding: "recovery re-runs the whole job; operators dedupe downstream today"
```

The assertion states what *should* be true (`runs == 1`). The `finding:`
states what *is* true instead, in words an operator can use.

## 2. Run it — the gap holds

```sh
shinari run crash-recovery-duplicates
```

```text
  ◆ FINDING exactly once
  => PASSED
```

Exit `0`. The check rendered as **FINDING**, counted separately, recorded in
`shinari-out/findings.md`:

```text
**Gapped**
- recovery re-runs the whole job; operators dedupe downstream today (check: exactly once)
  - observed: assert failed: expected 2 == 1
```

Your suite is now *living documentation of how the system fails* — and it is
green, so people keep watching it.

## 3. Fix the system — the ledger bites

Simulate the fix: delete the second `echo` line (the duplicate re-run) and run
again:

```text
  ✗ exactly once — finding now passes — the gap "recovery re-runs the whole job;
    operators dedupe downstream today" was fixed; promote this to a hard assertion
  => FAILED
```

Exit `1`. CI goes red on a **good** event — deliberately. A finding that
passes silently would rot into a stale claim; instead the harness demands you
delete the `finding:` line, turning the check into a hard assertion forever.

## 4. Promote it

Remove the `finding:` key:

```yaml
  - run: assert
    with:
      of: "${.runs.value}"
      equals: 1
    desc: "exactly once"
```

Run once more: green — and this time `exactly once` is a regression tripwire,
not a documented gap.

## The lifecycle you just executed

```text
gap discovered ─▶ finding: declared ─▶ suite GREEN, gap visible in the ledger
                                   │
                       product fixes the gap
                                   │
                  suite RED: "promote this" ─▶ finding: removed ─▶ hard assertion
```

That loop — *green until the product changes* — is what Shinari exists to
run. The full reasoning lives in
[Why a findings ledger](/concepts/findings-ledger/).
