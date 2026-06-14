---
aliases: [/explanation/event-gating/]
title: Determinism & event gating
description: Why faults fire on observed events, never timers — and what that buys the ledger.
weight: 20
---

The classic resilience test kills a worker "about three seconds after
submitting the job, which is usually when it's mid-task." It passes for
months. Then CI gets slower, the job starts later, the kill lands *before*
the task starts — and the test flakes. Or worse: it silently stops testing
the interesting moment and keeps passing.

## Time is not an event

What you mean is never "at t+3s". It is "when the job is RUNNING", "the
instant the service logs `stream started`", "once the queue depth exceeds
100". Those are **observations**, and Shinari makes them the only way to
sequence a timeline:

```yaml
- run: wait_until
  with:
    probe:
      run: docker.logs
      with: worker-a
    matches: "stream started"
    timeout: 30
- run: docker.kill
  with: worker-a
```

`wait_until` polls a provider probe until the condition holds, then releases
the timeline. The gate is engine; the observation is a provider. The kill
lands at the same *system state* every run, on every machine, at any CI
speed.

## Why the ledger needs this

A finding is a claim: "under fault X, the system does Y." That claim is only
worth recording if re-running the scenario reproduces it. Timer-based
injection makes the fault land in a different system state each run — the
claim becomes "under fault X *at some point*, the system *sometimes* does Y",
which is noise. Event gating is not an ergonomic nicety; it is the
load-bearing property that makes findings reproducible, hence credible,
hence worth keeping green.

## The two-clock rule

`sleep` still exists — for waits that are *genuinely* about time: a TTL
expiring, a scheduler tick, a lease timeout. The discipline:

- waiting for the **system** → `wait_until` (an observation)
- waiting for **time itself** → `sleep` (and say why in `desc:`)

If you cannot name the event you're waiting for, you haven't understood the
scenario yet — which is usually the more important discovery.

## Reproducibility by construction

Event gating is one of two pillars; the other is **pinned providers**
(`shinari.lock.yml`) so the verbs behave identically across machines. State
that could drift between runs — state files, persisted fixtures — is
deliberately absent: every run is ephemeral, built up and torn down whole.
