---
aliases: [/explanation/provider-model/]
title: The provider model
description: How Shinari configures providers, what it refuses, and the two ways a capability gets implemented.
weight: 40
---

A provider is a named, namespaced bundle of capabilities; each capability is
a verb addressed `<provider>.<verb>`. Namespacing is mandatory — collisions
are impossible by construction.

## How configuration works

- **Configure once, reuse everywhere.** Endpoints, credentials, and compose
  files live in one `providers:` block; every verb inherits them. No per-step
  endpoint repetition.
- **Named instances.** The configured name *is* the namespace: configure one
  type twice (`appA`, `appB`) and you get two verb namespaces, two deployments
  addressable from one scenario.
- **Pinned versions in a committed lock file** (`shinari.lock.yml`) →
  reproducible runs.
- **`init` before `run`** to resolve what's declared.

## Deliberately refused

- **State.** No desired-state reconciliation, no drift, no state surgery.
  Shinari runs are ephemeral; there is nothing to reconcile and no file to
  corrupt.
- **A resource meta-language.** No `count`/`for_each` or HCL-style graph
  wiring. Expressions are jq (the same language as `read:`/`capture:`); the
  moment logic outgrows a jq expression it belongs in a script behind
  `exec.run`, where it can be tested like code.
- **Schema gymnastics.** Verb arg specs are name/type/required — enough for
  `validate` to catch typos before a run, nowhere near a type system.

## Two implementations, one model

| kind | written in | when |
|---|---|---|
| built-in native | Go, compiled in | the arsenal: `docker`, `toxiproxy`, `net`, `http`, `exec` |
| composed | YAML macros over other verbs | your domain vocabulary — most teams never need more |

The taste test that routes authorship: *can it be composed from existing
verbs?* Yes → composed provider. No → native verb. Author discipline
("don't forget the exactly-once assertion") is never a verb — it's a
`validate` rule.

The `sdk` package is the isolation seam every native provider implements, so
adding k8s or podman later is a new provider: scenarios using `docker.*`
change while the engine does not.
