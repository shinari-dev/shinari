---
title: Build a provider
description: From YAML macros to a native Go provider against the SDK, and when to choose which.
weight: 10
---

Every capability in Shinari is a namespaced verb resolved from a provider.
There are three ways to add verbs, in escalating order of effort. **Start at
the top**; most teams never need to leave it.

## The taste test

> *Can it be composed from existing verbs?*
> **Yes** → a composed provider (YAML, this page, first section).
> **No** → a native provider (Go against the SDK, second section).
> *"Make scenario authors remember to do X"* → neither; that's a `validate`
> rule, not a verb.

## Level 1: composed provider (YAML, no Go)

A `kind: Provider` resource declares macros over existing verbs. This is
where domain vocabulary lives (`app.submit`, `app.await`), built on the
`http`/`exec` primitives:

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
```

The full walkthrough is in
[Compose a domain provider](/how-to/compose-a-provider/). Reach for Level 2
only when the capability *cannot* be expressed with existing verbs: a new
protocol, a new injection mechanism, a tool with no CLI.

## Level 2: native provider (Go, against the SDK)

A native provider is one Go type implementing `sdk.Provider`. The `sdk`
package is the entire contract, and a provider never imports the engine:

```go
package sdk

type Provider interface {
    Type() string
    Configure(cfg map[string]any) error
    Verbs() []VerbSpec
    Run(ctx context.Context, verb string, args map[string]any) (VerbResult, error)
}
```

### Declare your verbs

Each `VerbSpec` tells the engine everything it needs for resolution,
validation, dry-run, and the verdict model:

| field | meaning |
|---|---|
| `Name` | local verb name, snake_case (`flush_cache`); the engine namespaces it with your instance name |
| `Kind` | `action` (mutates), `probe` (observes), `assertion` (judges); drives dry-run skipping, steadyState re-runs, and where `finding:` is allowed |
| `SideEffects` | `true` for anything that mutates the system; powers composed-verb kind inference |
| `Effect` | the fault this verb injects: `EffectOutage` (drops/blocks work), `EffectDegradation` (slows it), or unset for non-faults; drives `fault.injected` tracking and the validate recovery rule, so a fault is recognized by its declaration, not its name |
| `Primary` | the arg bound when a step writes `with: <scalar>` shorthand |
| `Args` | name/type/required triples, enough for `validate` to catch typos before a run |

### A complete example

A provider for a fictional message broker, with one fault and one probe:

```go
// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package brokerp

import (
    "context"
    "fmt"

    "github.com/shinari-dev/shinari/sdk"
)

type Provider struct {
    adminURL string
}

func New() sdk.Provider { return &Provider{} }

func (p *Provider) Type() string { return "broker" }

func (p *Provider) Configure(cfg map[string]any) error {
    if u, ok := cfg["adminUrl"].(string); ok {
        p.adminURL = u
    }
    if p.adminURL == "" {
        return fmt.Errorf("broker provider needs config adminUrl")
    }
    return nil
}

func (p *Provider) Verbs() []sdk.VerbSpec {
    return []sdk.VerbSpec{
        {
            Name: "drop_partition", Kind: sdk.KindAction, SideEffects: true,
            Effect: sdk.EffectOutage, // a fault that can drop in-flight work
            Primary: "topic",
            Args: []sdk.ArgSpec{
                {Name: "topic", Type: "string", Required: true},
                {Name: "partition", Type: "number"},
            },
        },
        {
            Name: "lag", Kind: sdk.KindProbe, Primary: "group",
            Args: []sdk.ArgSpec{{Name: "group", Type: "string", Required: true}},
        },
    }
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
    switch verb {
    case "drop_partition":
        // ... call the broker admin API ...
        return sdk.VerbResult{Value: "dropped"}, nil
    case "lag":
        // ... read consumer lag; return a structured value so read:/capture: work
        return sdk.VerbResult{Value: map[string]any{"messages": 42}}, nil
    }
    return sdk.VerbResult{}, fmt.Errorf("broker has no verb %q", verb)
}
```

Contract notes:

- **`Configure` is called once** per configured instance; fail fast there,
  not on first `Run`.
- **Return structured `Value`s** (maps, slices, numbers); that's what makes
  `read:`, `capture:`, and `assert` work on your results. Put raw logs in
  `Output`.
- **Errors are step failures.** Make the message name the verb and the
  target: it lands verbatim in the console, the reports, and the findings
  ledger.
- A relative-path-resolving provider receives `projectDir` in its config map;
  anchor file paths on it, not on the process cwd.

### Register and use it

A provider **self-registers** its type name from an `init()`, so nothing in
the engine has to know it exists (the same inversion as `database/sql`
drivers). Put this in your provider package:

```go
func init() { sdk.Register("broker", New) }
```

The engine resolves configured types through that registry, so the only thing
left is to make sure your package is linked into the binary by importing it
(for the built-ins that is `providers/all`, which the CLI imports; for your
own provider, blank-import it from your `main`):

```go
import _ "example.com/me/brokerp" // runs init(), self-registers "broker"
```

No change to `core` is ever required to add a provider. Type names are a flat
namespace: registering one that is already taken panics at startup, so a
collision surfaces loudly instead of being decided by import order. Then
configure an instance like any other:

```yaml
providers:
  broker:
    config:
      adminUrl: http://localhost:9644
```

```yaml
method:
  - phase: "Drop the hot partition mid-consume"
    steps:
      - run: broker.drop_partition
        with:
          topic: orders
          partition: 3
```

### Test it like the built-ins do

Unit tests never need real infrastructure: fake the upstream (an
`httptest.Server`, a stub binary) and assert on what your provider sends.
Scenario-level tests register the provider under a fake type and run the
engine against scripted values; see `core/engine/engine_test.go` for the
pattern.

### Document it

A built-in provider ships a reference page under
`docs/content/reference/providers/<type>.md`. The pages follow one shape so a
reader learns every provider the same way:

1. **Intro** — one paragraph: what it does, and when to reach for it.
2. **Config block** — the `providers:` YAML, then prose describing each config
   key and its default.
3. **`## Verbs`** — one `### <verb> (<kind>[, <effect>])` section per verb
   (group verbs that share an arg shape, e.g. `### post / put / delete`). Each
   section has:
   - a one-line description of what the verb does;
   - an **arg table** with columns `arg | type | req | description`, marking the
     primary arg; write `No args.` when there are none;
   - a **`**Returns**`** line describing the `value` shape, every `meta` key the
     verb sets, and what `output` holds;
   - a **minimal runnable example**.
4. **Narrative sections** (optional) — any cross-cutting behavior that does not
   belong to one verb (status handling, lifecycle notes) as its own `##`
   section below the verbs.

The arg table mirrors your `Args` declaration and the `**Returns**` line mirrors
what `Run` puts in the `VerbResult`; keep them in step when either changes.

## Going deeper

Engine internals (the three-package architecture, the result/event
contract, design principles and non-goals) live in
[`DEVELOPERS.md`](https://github.com/shinari-dev/shinari/blob/main/DEVELOPERS.md).
