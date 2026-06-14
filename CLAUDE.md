# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Shinari is a **resilience integration testing** framework. Tests are YAML scenarios; a Go engine
brings up a real system, injects controlled deterministic faults, and asserts how it survives.

## Commands

```sh
go build -o shinari ./cli          # build the CLI (binary name: shinari, alias shi)

go test ./...                      # all tests (pure unit tests, no infra required)
go test ./core/engine/             # one package
go test ./core/engine/ -run TestRunScenario   # one test
go vet ./...

./shinari -C examples/quickstart validate   # static checks, no run
./shinari -C examples/quickstart list       # scenarios grouped by suite
./shinari -C examples/quickstart run        # execute; exit code = verdict
```

Tests are hermetic — there are no `//go:build integration` tags or skipped infra tests; the whole
suite runs offline. Reports from `run` land in `shinari-out/` (gitignored).

## Architecture

The pipeline is **parse → resolve → execute → verdict → emit**, split across top-level packages
with a strict dependency direction (every arrow points down: `cli → core → sdk`, `cli → providers → sdk`):

- **`core/`** — the engine library. It emits a structured `RunResult` + a typed event stream and
  **never prints and never calls `os.Exit`**. Core also never reads the environment (env like
  `KEEP_UP=1` is mapped onto `engine.Options` by the CLI). Core is **provider-agnostic**: it imports
  no concrete provider.
- **`cli/`** — the front end and **composition root**: argv parsing, all rendering (console, TSV,
  JSON, JUnit XML, journal, findings report), and the mapping of verdict → exit code. It decides which
  providers ship in the binary by blank-importing `providers/all` (`wiring.go`); it is the only
  package that links both `core` and the concrete providers.
- **`sdk/`** — the provider contract (`Provider`, `VerbSpec`, `VerbResult`, `Kind`) **and the
  registration seam** (`Register`/`Factory`, the database/sql-style driver table). Providers link only
  this package, never the engine.
- **`providers/`** — the five native providers (`execp`, `httpp`, `dockerp`, `toxiproxyp`, `netp`),
  each linking only `sdk` (plus the `utils/conv` leaf) — exactly the shape a third-party out-of-tree
  provider takes. Each **self-registers** its type from an `init()` (`sdk.Register("docker", New)`);
  `providers/all` blank-imports them so a binary loads the built-in set with one import. **Adding a
  provider needs no core change** — write the package, self-register, and add one line to
  `providers/all` (or have your own binary blank-import it).
- **`utils/conv/`** — a dependency-free leaf of small value helpers (`ToFloat`, `ToString`,
  `Truncate`) shared by core and the providers.

### core sub-packages

- `model/` — YAML types. Resources are recognized **by their `apiVersion`/`kind` header, not by
  filename** (`ParseHeader`). `Step.With` stays a `yaml.Node` so scalar/list/map `with:` shorthands
  survive until interpolation.
- `discover/` — walks the project tree, parses every `.yml`/`.yaml`, collects resources by kind. A
  recognized header with a malformed body is an error, not a silent skip; unrecognized files are
  silently ignored.
- `registry/` — holds the configured provider instances of a run and resolves each step's `run:`
  (`<instance>.<verb>`, or an unprefixed language builtin) against the union of their verb specs.
  It resolves native types through `sdk.Factory` and never imports a provider, so core stays
  provider-agnostic; providers self-register into the `sdk` table, and tests register fakes there too.
- `engine/` — `Run` → `RunScenario`. The scenario lifecycle is
  **setup → steadyState (gate) → method phases → steadyState (recovery) → verify → teardown (always)**
  (`executor.go`). `events.go`/`result.go` define the stream and result; `Reduce` rebuilds the result
  from events alone (the design constraint that Result is the stream's deterministic reduction).
- `interp/` — `${...}` variable/capture interpolation. `jqx/` — gojq transforms (the `read:` key).
- `builtins/` — the unprefixed language verbs: `assert`, `sleep`, `wait_until`, `background`,
  `stop_background`, plus the shared assert-operator set.
- `validate/` — static checks producing severities (`Error`/`Warning`).

### Two concepts that drive most of the design

**The findings ledger.** A step with `finding:` marks a check as a *known, expected* failure.
When that check fails it is recorded as `FINDING` and the scenario **stays green**. When it starts
*passing* (the gap was fixed), the run flips to `FAILED` with "promote this to a hard assertion". See
`judge` in `executor.go`.

**Verb `Kind` (action/probe/assertion).** Kind drives three behaviors: dry-run skips actions,
steadyState recovery re-runs probes only, and the verdict split.

**Verb `Effect` (outage/degradation/none).** A verb declares whether it injects a fault, orthogonal
to Kind. The engine tracks `Effect != none` actions in `method` as injected faults, and validate's
recovery rule keys off `EffectOutage` — both read it from the spec instead of matching verb names, so
a third-party fault verb participates with no core change. Composed verbs inherit the strongest
`Effect` of their leaves, and a step may set `effect:` to declare a fault injected through a
polymorphic verb (`exec.run` running `tc`/`iptables`, `http.post` to a chaos endpoint) — the same
per-step override pattern as `kind:`.

### Verdicts → exit codes

`PASSED`→0, `FAILED`→1, `ERRORED` (setup failed)→2, `INCONCLUSIVE` (steadyState failed before
method)→3. CLI **usage** errors exit `64` (EX_USAGE) to stay distinct from verdicts.

## Providers are composable in two ways

1. **Native** Go providers implementing the `sdk.Provider` interface (the five built-ins).
2. **Composed** providers: `kind: Provider` YAML macros over other verbs, zero Go — see
   `examples/quickstart/providers/jobstore.yml`. A composed verb declares `params:` and a `do:`
   (sequence) or `probe:` (single observation).

## Conventions

- Source files carry SPDX headers (`Apache-2.0`, `© The Shinari Authors`).
- Project layout (`scenarios/<suite>/<name>.yml`, `providers/`, `scripts/`, `assets/`) is a
  convention only — never mandated; discovery walks the tree.
- Docs (`docs/`) are a Hugo site organized by Diátaxis (tutorials / how-to / reference / concepts).
