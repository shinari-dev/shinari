---
aliases: [/explanation/findings-ledger/]
title: Why a findings ledger
description: Known gaps stay green and documented, then flip red the day they are fixed. How a Shinari suite grows with your code instead of decaying.
weight: 10
---

Every team that runs serious failure testing discovers the same things: the
system loses messages during leader election, duplicates work on crash
recovery, hangs when DNS flaps. And every team stores those discoveries the
same two broken ways.

## The two broken ways

**The wiki page.** Findings go into a document. The document is true the day
it's written and decays from then on. Nobody re-verifies it; two quarters
later half the gaps are fixed, half the workarounds are wrong, and nobody
knows which half.

**The red test.** Findings stay as failing tests, "so we don't forget."
The suite is now permanently red. Humans cannot watch a red suite; they
learn, rationally, to ignore it. The day a *real* regression appears, it
drowns in the expected failures.

## The contract instead

Shinari makes a finding an **executable, self-maintaining contract**:

```yaml
- run: assert
  with:
    of: "${.outputs.total.value}"
    equals: 1
  desc: "exactly once"
  finding: "recovery re-runs the whole job; operators dedupe downstream today"
```

The assertion states what *should* hold. The `finding:` states what holds
instead. The harness verifies both, every run:

- While the gap exists, the check fails (**as declared**), renders as
  `FINDING`, and the run stays green. The suite remains a signal.
- The day the gap is fixed, the check *passes*, which **fails the run**,
  with one instruction: *promote this to a hard assertion*. Delete the
  `finding:` line and the check becomes a permanent regression tripwire.

CI goes red only on real change: a regression (something that held no longer
holds) or a fix (something documented as broken no longer is). Both deserve
a human's attention. Nothing else does.

## Why green matters more than red

The counterintuitive bet: an expected failure should keep CI **green**.
The alternative ("warning" states, allowed-failure lists, quarantined
suites) all converge on the same outcome: a category of signal that everyone
filters out. Verification you ignore is not verification. The ledger works
because there is exactly one red, and it always means *act*.

## The suite as documentation

Run after run, the findings report accumulates the honest answer to "how
does this system fail?", per scenario: what was **injected**, what **held**,
what **gapped**, with the operator workaround in the narrative. It cannot go
stale, because every line is re-proven on every run. This is how a Shinari
suite grows with your code instead of decaying: the resilience tests are the
front door, and the ledger is what keeps them honest over time.
