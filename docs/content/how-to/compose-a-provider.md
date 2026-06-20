---
title: Compose a domain provider
description: Give your system under test its own vocabulary (submit, await, count) with zero Go.
weight: 30
---

**Goal:** scenarios that read `app.submit` / `app.await` instead of raw
`http.post` plumbing.

## 1. Declare a kind: Provider resource

Create `providers/app.yml`:

```yaml
apiVersion: shinari/v1
kind: Provider
name: app
verbs:
  submit:
    params: [job, "inputs?"]
    do:
      - run: http.post
        with:
          path: "/jobs/${.params.job}"
          form: "${.params.inputs}"
        capture:
          id: ".id"
  await:
    params: [of, timeout]
    do:
      - run: wait_until
        with:
          probe:
            run: http.get
            with:
              path: "/jobs/${.params.of}"
          read: ".state"
          in: [SUCCESS, FAILED]
          timeout: "${.params.timeout}"
  count:
    params: [job]
    probe:
      run: http.get
      with:
        path: "/jobs?type=${.params.job}"
      read: ".items | length"
```

> A trailing `?` marks a param optional, and it must be **quoted**
> (`"inputs?"`): bare `inputs?]` is invalid YAML flow syntax.

## 2. Configure it

In `project.yml`, point an instance at the definition and give it config:

```yaml
providers:
  http:
    config:
      baseUrl: http://localhost:8080
  app:
    use: ./providers/app
```

The instance name (`app`) becomes the verb namespace: `app.submit`,
`app.count`. Configure the same definition twice under two names (`appA`,
`appB`) to talk to two deployments.

## 3. Use the vocabulary

```yaml
method:
  - phase: "Submit a long job"
    steps:
      - run: app.submit
        with:
          job: sleep
          inputs:
            seconds: 30
        as: job
verify:
  - run: app.await
    with:
      of: "${.outputs.job}"
      timeout: 420
  - run: app.count
    with: sleep
    as: total
  - run: assert
    with:
      of: "${.outputs.total.value}"
      equals: 1
    desc: "exactly once"
```

## What the engine infers for you

- **Kind**: a verb whose body mutates (`http.post`) is an *action*; a body
  ending in `assert` is an *assertion*; otherwise it's a *probe*. Kind drives
  dry-run, steadyState re-runs, and where `finding:` is allowed.
- **Args**: `params:` become the verb's arg spec: required unless marked
  `"name?"`, first param doubles as the scalar shorthand
  (`with: sleep` ≡ `with: { job: sleep }`).
- **Scoping**: `${.params.NAME}` references and body captures are macro-local;
  they never leak into the scenario's `outputs` namespace.

## Rules to know

- Composed verbs may call native verbs and builtins freely, but another
  composed verb only **one level deep**: `validate` rule 4 rejects deeper
  nesting. Vocabulary, not an inner platform.
- If a step needs logic YAML can't express, don't fight it: wrap a script with
  `exec.run` inside the verb body.
