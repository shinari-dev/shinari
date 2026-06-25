# Spec: Persistent Ledger, Run History, and the Live TUI

*Status: proposed. Decided in an ideation pass on 2026-06-25, grounded against the
shipped engine. This spec records the architecture decision and its reasoning so the
build does not relitigate it. Companion to `product-discovery-brief.md` (which frames
the opportunity); this file is the design of record for the three features.*

*Markers: **[fact]** (cited from code), **[decision]** (the choice made), **[deferred]**
(explicitly out of scope for now).*

---

## 1. Scope

**In scope:** make the findings ledger durable (a committed golden artifact), build
run-over-run history, and ship a live/replay TUI.

**[rejected]** a `shinari bisect` command. Value not established and the determinism it
would lean on is only partial (see §8); if revisited it is external orchestration
(`git bisect run shinari run`), never a Shinari subcommand.

**[deferred]** a custom PR-comment bot, a composed-provider registry, and
resource-exhaustion fault breadth.

The three in-scope features share one load-bearing prerequisite (finding identity) and
one substrate (the event stream), which is why they are specified together.

---

## 2. Governing principles

Two invariants govern every decision below. Both extend the existing engine contract
(core never prints, never `os.Exit`, never reads env, imports no concrete provider).

**P1 — Core never assumes a VCS.** Shinari reads and writes files at paths the user
controls and returns a verdict as an exit code. It never shells out to git, never
imports a VCS client, never assumes one exists. Whether an artifact is committed,
cached in CI, or discarded is the user's decision, identical behavior either way. A
feature that only works because the user happens to use git is not a Shinari feature.

**P2 — Own your origin schema, rent the standards at your edges.** The source of truth
stays a custom format Shinari controls and that models its domain losslessly. Industry
standards (JUnit, SARIF, OpenTelemetry) are projections emitted at consumption
boundaries, never the format `Reduce` reads. This is already how `junit.xml` ships
**[fact]** (`cli/render/files.go`): a standard at the CI boundary, not the native
result.

---

## 3. Decision 1 — History is `Reduce` folded over many runs

The event journal already exists: every run writes `journal.jsonl`, one JSON object per
line, and "the journal IS the event stream" **[fact]** (`cli/render/files.go:56`,
`cli/cmd_run.go:99`). `RunResult` is the deterministic reduction of that stream
**[fact]** (`core/engine/events.go:37`). History therefore adds **no new core concept**:
it is `Reduce` applied to many runs instead of one.

Three distinct artifacts, not to be conflated:

| artifact | cardinality | role | format |
|---|---|---|---|
| **Journal** | one per run | full event stream, replayable source of truth, feeds the TUI | custom NDJSON (`Event`) |
| **Run record** | one per run | compact `{runId, time, label, verdict, findings[]}`, the durable history unit | custom, small |
| **Golden ledger** | one, current | the expected findings a run is compared against (snapshot pattern, `-u` to update) | human-first YAML |

**[decision]** History merges by **folding each run's journal into a per-run finding
projection, then sequencing the results keyed by finding identity.** It is not a raw
concatenation of event streams: events from different runs must never interleave (each
carries `Time` **[fact]** `core/engine/events.go:40`, but the unit of history is the
run, not the event). The golden ledger is a separate kind of thing from history: an
*expectation* a run is compared against, not a *record* of what happened. Keep them
distinct.

---

## 4. Decision 2 — Two output stores: ephemeral scratch vs durable history

Today `writeReports` writes five fixed filenames into a flat `shinari-out/` via
`os.Create`, which truncates **[fact]** (`cli/cmd_run.go:91-103`, default `out` at
`:34`). **A second run clobbers the first; the disk holds only the latest run.** This
is **correct** for what it is today: an ephemeral CI artifact you upload.

**[decision]** Do not change the overwrite. History is *additive*, a second store:

- **Scratch (`shinari-out/`, unchanged):** latest run only, overwritten, gitignored, the
  CI artifact. The full journal lives here for the TUI to replay the last run.
- **History (new, opt-in, append-only):** one compact run-record appended per run, never
  overwritten. `shinari log` and the trend view fold over this.

The run-record needs a filing label so records order and do not collide: a **run id**, a
wall-clock timestamp. This does not violate determinism because the run id is a *filing
label, never a verdict input*. Content (findings, verdict) stays the pure reduction of
the event stream; only the folder it lands in carries the timestamp. Observational, not
decision-making.

---

## 5. Decision 3 — Finding identity is a step-level property, optional, derived by default

Identity is the load-bearing prerequisite for all three features (golden matching,
history trend, TUI run-to-run alignment). A finding is just a step with `finding:` set
**[fact]** (`core/engine/executor.go:359`), so identity belongs to the **step/check**,
not to findings alone. Scoping it to findings would make a finding *lose* its identity
the moment it is promoted to a hard assertion, which is exactly backwards.

**[decision]**

- **Derived structural fingerprint by default** (over scenario + section + verb + target
  + operator + operand), with an explicit `id:` override on any step. Same
  derived-default-plus-per-step-override pattern as `kind:` and `effect:` **[fact]**
  (`CLAUDE.md`, `core/model/scenario.go`).
- **The narrative is human prose, separate from the id.** Today identity is keyed by the
  narrative string, so fixing a typo breaks history. The id must be structural or
  explicit; the narrative must stay free to reword.
- **`id` is optional, not required.** Validate emits a **Warning** (not an Error) when a
  finding has no explicit id. The derived fingerprint is stable across the comparison
  GitHub actually makes (a PR that *fixes* a gap changes the code, not the scenario, so
  verb/target/operator/operand are unchanged). An explicit id only earns its keep when a
  PR edits the *scenario* itself (retunes a threshold, renames). Pre-release with no
  users, requiring an id pays a friction cost up front for a churn problem not yet
  observed; tightening to required-on-findings is a one-line validate change deferrable
  until real PRs prove derived ids too noisy.

---

## 6. Decision 4 — No pluggable history store. Emit a stream.

The temptation is a `HistoryStore` plugin interface so a future cloud product can store
history elsewhere. **[decision] Rejected.** The right seam is not a storage interface, it
is the artifact itself, and it already exists at the right altitude:

- Output is already an `io.Writer`, not a path **[fact]** (`render.Journal(w io.Writer,
  ...)`, `ResultsJSON`, `TSV` in `cli/render/files.go`).
- The live side is already an `Emitter` with `Multi(...)` fan-out **[fact]**
  (`core/engine/events.go:47-74`).

So filesystem is `w = os.File`, stdout/pipe is another writer, and a cloud agent is just
another `Emitter` subscribed to the same stream the TUI subscribes to.

**Plugin-seam test** (derived from why `sdk.Register` for providers earns its keep): a
seam is justified only when the variant set is **open, third-party authored, and varies
per use**. Providers pass all three; a history store fails all three (closed set, all
owned by Shinari, invariant per scenario). A storage interface would also drag auth,
endpoints, and retries into the platform-free core, breaking P1. The SaaS future *wants
Shinari to know less*: a dumb agent emits a faithful versioned stream and forgets; the
platform owns storage, indexing, and RBAC. The only generalization worth doing now is
letting the CLI aim the journal writer at stdout or a pipe, a few lines, not a framework.

---

## 7. Decision 5 — Formats: custom NDJSON origin, standards as exporters

**[decision]**

- **Native source of truth: the custom, versioned NDJSON `Event` stream.** Add a
  `schemaVersion` field, since the TUI and history now depend on its shape. It is
  greppable, diffable in a PR, tailable by a thin TUI, dependency-free, and models
  findings losslessly because Shinari defines it. `Reduce` reads this and nothing else.
- **SARIF as a renderer** (`--format sarif`), alongside JUnit. SARIF is the one standard
  that matches the findings domain (results + rules + levels + fingerprints), and GitHub
  code scanning ingests it natively, which delivers the PR-annotation surface with no
  custom bot and no git dependency. Always emit the derived fingerprint as
  `partialFingerprints` so cross-run correlation works zero-config.
- **OpenTelemetry as an exporter, not the origin.** OTel fits the *timeline* (run =
  trace, steps = spans, fault/gate = span events, latency windows = histograms) but not
  the *differentiator*: OTel span status is only Ok/Error/Unset, so a FINDING and
  "finding now passes" have no native representation and degrade to custom attributes.
  Making OTLP the source of truth would also force a heavy OTel SDK dependency into core
  (breaking the one-binary stance), fight determinism (random span ids vs stable ids for
  replay/diff), and still require a separate human-diffable format anyway, which proves
  OTLP is an export. OTel *is* the right choice for the cloud product's *ingestion API*
  (the backend ecosystem already speaks it), but that is a statement about the platform's
  API, not Shinari's native schema. The agent keeps local NDJSON and also runs an OTLP
  exporter aimed at the collector. Both true at once.
- **Golden ledger: human-first YAML.** It is read by humans in a PR diff; diffability and
  comments beat tooling interop, and YAML matches the scenario DSL. Borrow SARIF's
  fingerprint for the identity field, but never make a human read SARIF in code review.

---

## 8. Determinism note — finding stability and golden churn

This is not a feature, it is a constraint the golden and history must respect. A
finding's verdict is a pure function of one bit (`judge`, `executor.go:356`), so its
stability reduces to whether the asserted value is deterministic. Two classes:

- **Behavioral findings** (status, row count, key presence, exit code) are discrete
  functions of system behavior, and `wait_until` gates on observed state not a timer
  **[fact]** (`executor.go:689`), so there is no ordering race. Stable; safe to commit
  to a golden.
- **Quantitative findings** (p99, errorRate, recovery latency) are wall-clock
  measurements reduced by `stats.Summarize` **[fact]** (`executor.go:673`,
  `utils/stats/stats.go`), and nearest-rank p99 over a small window is effectively the
  max of a few noisy samples. They can flip run-to-run on the *same* commit.

**Implication for this spec:** a quantitative finding committed to the golden will churn
the golden and produce noisy history. Event-gating removes *ordering* nondeterminism, not
*measurement* nondeterminism. The golden and history features do not solve this; they
must simply not pretend a noisy finding is a clean signal. Hardening it (baseline-margin
assertions, a `characterize` mode, scenario `--repeat N`) is **[deferred]** follow-up
work, tracked as an open question (§10), not part of this build.

---

## 9. Build order

1. **Finding identity first** (§5): the structural fingerprint and optional `id:`.
   Everything else inherits it.
2. **Output layout** (§4): per-run isolation plus the append-only history store, beside
   the unchanged scratch dir.
3. **Golden ledger** (§3): YAML expectation, compare on run, `-u` to update; the snapshot
   must be an *input expectation*, never engine memory, so `RunResult` stays a pure
   reduction.
4. **History view** (`shinari log`) folding run-records.
5. **TUI**: a live tail of the existing `Emitter` stream; replay is `Reduce` over a saved
   journal. No new data format.
6. **Exporters** (§7): SARIF for the GitHub surface; OTLP when the cloud path is real.

---

## 10. Open questions

- **Finding-identity stability in practice.** Does the derived structural fingerprint
  actually survive normal scenario editing, or do real PRs churn it enough to force
  required ids? Instrument before deciding.
- **History retention.** Keep full journals (rich replay, large) or only compact
  run-records (cheap, no deep replay of old runs)? Default to compact records as the
  durable unit; add journal compaction only if deep historical replay is demanded.
- **Quantitative finding churn.** A latency/percentile finding can flip on the same
  commit (§8) and will churn the golden and history. Measure the same-commit flip rate of
  a representative quantitative finding before deciding whether the golden needs
  baseline-margin assertions or a `characterize` mode. Out of scope for this build, but it
  is the first follow-up once history is real.
