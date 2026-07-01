# Developing Shinari

Engine internals for contributors. If you only want to *extend* Shinari with a
new provider, read the website's [Developers guide](https://shinari.dev/developers/)
first: most extensions are YAML composed providers or a single Go file against
`sdk/`, and never touch the engine.

## Repository layout

One Go module, three packages with a strict dependency direction:

```
cli ──▶ core ──▶ sdk ◀── (future provider plugins)
```

| package | owns | never |
|---|---|---|
| `core` | parse, resolve, execute, verdict, the findings-ledger logic; emits *data* | reads argv, prints, assumes a TTY, calls `os.Exit`, owns an exit code |
| `cli` | argv, the streaming console, report files, verdict to exit-code mapping | reaches into engine internals |
| `sdk` | the provider contract: `Provider`, `VerbSpec`, `ArgSpec`, `VerbResult` | imports anything above it |

A provider links **only** `sdk`, never the executor, reporter, or discovery
walker. A REST API or web UI later is a fourth thin module over `core`, with
zero core change. The boundary is enforced by a test
(`cli/main_test.go: TestCoreNeverImportsCLIOrExits`): if `core` imports a CLI
package or calls `os.Exit`, the suite fails.

### core, package by package

- `core/model`: typed resources (Project, Scenario, Provider) and the step
  envelope; recognition by `apiVersion`/`kind` header.
- `core/discover`: walks a project tree, collects resources by kind; a
  recognized-but-malformed file is an error, never a silent skip.
- `core/interp`: `${...}` string interpolation over vars and captures. No
  expression language, by design.
- `core/jqx`: gojq wrapper for `read:`/`capture:` expressions.
- `core/registry`: configured provider instances; verb resolution
  (`<instance>.<verb>`); composed-provider macro expansion, kind inference,
  nesting bound.
- `core/builtins`: the language verbs' specs and the closed assert-operator
  set (execution of `wait_until`/`background` lives in the engine: they need
  the timeline).
- `core/engine`: the executor (sections, phases, captures, teardown), the
  event stream, verdict resolution, findings logic, run lifecycle, and
  the event-to-Result reducer.
- `core/validate`: the ten static rules.
- `providers/*`: the built-in native providers, each linking only `sdk` (plus
  the `utils/conv` leaf). They live outside `core` so core stays
  provider-agnostic, and each self-registers its type from an `init()` via
  `sdk.Register`. `providers/all` blank-imports them; the CLI imports that to
  load the built-in set. A new provider needs no core change.
- `utils/conv`: dependency-free value helpers (`ToFloat`, `ToString`,
  `Truncate`) shared by core and the providers.

## The result & event contract

The package split only pays off if the data crossing `core`'s edge is rich
enough that every front end renders **without re-running anything**:

- **`engine.RunResult`**: terminal outcome, fully structured: per-check
  verdicts (PASS/FAIL/SKIP/FINDING), scenario verdicts, findings with
  narratives, timings, injected/held/gapped.
- **Event stream**: typed events emitted live (`scenario.started`,
  `step.*`, `fault.injected`, `gate.observed`, `finding.recorded`, ...).
  Append-only, ordered; `RunResult` is its deterministic reduction
  (`engine.Reduce`, covered by a round-trip test).

Acceptance test for any contract change: could a front end holding *only*
these types emit the JUnit XML **and** the findings report? If not, widen the
contract; never let a front end reach into engine internals.

The CLI renderings (`results.json`, `junit.xml`, `results.tsv`,
`journal.jsonl`, `findings.md`) are all produced in `cli/render` from this
contract; `core` writes no files.

## Design principles

1. **Capabilities come from providers; the engine stays small.** Everything
   that touches a system is a provider verb.
2. **No imposed structure.** Recognition by content marker, discovery by
   walking. Layout is convention.
3. **Declarative-first, escape-hatch-always.** YAML for the common case;
   `exec.run` guarantees no wall.
4. **Fail fast and clear.** Typed model plus `validate` before any run; every
   error names file, scenario, step, reason.
5. **Reproducible by construction.** Event gating (never timers) plus pinned
   providers (lock file).
6. **Stateless.** Runs are ephemeral. No state file, no reconciliation, no
   drift.
7. **Two ways to extend, no cliff.** Built-in native providers (Go) for the
   common arsenal; composed providers (YAML) for domain vocabulary built from
   existing verbs.

**The taste test** that routes new capability: *can it be composed from
existing verbs?* Yes: composed provider (YAML). No: native verb (Go).
"Make authors remember to do X": neither, that is a `validate` rule.
Guarantees enforced by lint stay enforced; guarantees enforced by convention
decay.

## Non-goals

- No expression language: logic goes to `exec`, where it can be unit-tested.
- No state, ever.
- No plugin host in v1 (the `sdk` package already isolates the contract).
- No second lifecycle runtime yet (compose only; the lifecycle-provider seam
  is ready for k8s).
- No two lifecycle runtimes in one scenario.
- No re-implementation of Toxiproxy/Docker/dnsmasq: providers drive them.

## Working on the code

```sh
go build ./...           # everything
go test ./...            # unit tests, no docker needed (providers are faked)
go vet ./...
go build -o shinari ./cli && ./shinari -C examples/quickstart run   # end-to-end
```

Conventions: SPDX headers on every source file (`The Shinari Authors`);
schema identifiers camelCase, verb names snake_case, kinds PascalCase;
tests fake providers through `sdk.Register` rather than touching real
infrastructure.
