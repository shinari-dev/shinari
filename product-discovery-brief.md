# Shinari — Product Discovery Brief

*Discovery pass through a PM lens. Goal: understand the product as it ships
today and find the features that serve adoption and ease of use. Claims are
marked **[fact]** (the code does this, cited) or **[inference]** (intent or
market judgment to validate). Grounded against the repository state of
2026-06-24.*

---

## 1. Project understanding

**What it is.** Shinari is a resilience integration testing framework. A test is
a YAML `kind: Scenario`; a Go engine brings up a real system, injects controlled
deterministic faults (kill a process, delay or partition the network, poison
DNS), and asserts how the system survives. Every run ends in a verdict; every
known weakness is tracked in a findings ledger that keeps CI green until the gap
is actually fixed. **[fact]** (`README.md:25-29`, `CLAUDE.md`).

**The problem it attacks.** Failure testing today lives in two broken places
(the project's own framing) (`docs/content/concepts/findings-ledger.md:14-23`):
a wiki page that silently decays, or a permanently-red test that humans learn to
ignore. Shinari's bet is that an expected failure should keep CI **green**, and
go red only on real change: a regression, or a finding that started passing.

**Core thesis (three load-bearing ideas).**

- **The findings ledger is the product.** A check with `finding:` is a known,
  expected failure: it renders `FINDING`, the run stays green, and the day it
  starts passing the run flips `FAILED` with "promote this to a hard assertion"
  (`docs/content/concepts/findings-ledger.md:27-49`,
  `core/engine/executor.go:356-380`). The docs say it outright: "That document is
  the product. The engine, the providers, the YAML are apparatus."
- **Determinism by event gating.** Faults fire on observed events (`wait_until` a
  probe sees the state), never on wall-clock timers, so a finding reproduces
  (`docs/content/concepts/event-gating.md:15-47`). Reinforced by pinned providers
  in a committed `shinari.lock.yml`.
- **Zero platform.** One Go binary, no agents, no chaos service, no cluster. Runs
  on a laptop or in CI (`README.md:104-107`).

**Architecture in product terms.** The pipeline is **parse → resolve → execute →
verdict → emit**, split with a strict downward dependency (`cli → core → sdk`,
`cli → providers → sdk`):

- **`core/`** — the engine. Emits a structured `RunResult` plus a typed event
  stream, never prints, never `os.Exit`, never reads env, imports no concrete
  provider. `RunResult` is the deterministic reduction of the event stream
  (`Reduce`, `core/engine/run.go:91-153`): renderers can rebuild the full result
  from a journal alone.
- **`cli/`** — the front end and composition root: argv, all rendering, verdict →
  exit code, and the one place that links both core and the providers.
- **`sdk/`** — the provider contract plus the `Register`/`Factory` registration
  seam (`sdk/sdk.go:10-33`).
- **`providers/`** — eleven native providers, each self-registering from
  `init()`. Adding a provider needs no core change.
- Two composition kinds: **native** Go providers and **composed** providers (YAML
  macros over other verbs, zero Go — `examples/quickstart/providers/jobstore.yml`).

**Verb taxonomy** drives behavior **[fact]** (`sdk/sdk.go:10-33`): **Kind**
(action / probe / assertion) gates dry-run (skip actions,
`core/engine/executor.go:305`), steadyState recovery re-runs, and the verdict
split; **Effect** (outage / degradation / none) declares fault injection
orthogonally, so a third-party fault verb participates with no core change. Both
support per-step overrides (`core/model/scenario.go:78-95`).

---

## 2. Existing features (shipped surface)

**Scenario lifecycle** (`core/engine/executor.go:64,114-170`) **[fact]**:
`setup → steadyState (gate) → method (named phases) → steadyState (recovery) →
verify → teardown (always)`. Scenario- and per-step timeouts
(`executor.go:92-101,316-324`); a `when:` jq guard skips a step as `SKIP`, not an
error (`executor.go:278-290`); `--dry-run` skips action-kind steps only
(`executor.go:305-308`); teardown always runs under `context.WithoutCancel` and
never changes the verdict.

**Native providers (11 providers, ~42 verbs)** **[fact]** (source-cited
inventory across `providers/*`):

| provider | verbs | notable faults / probes |
|---|---|---|
| `docker` (12) | up, down, kill, stop, start, pause, unpause, logs, ps, exec, disconnect, connect | compose lifecycle + profiles + health wait; process faults (kill/stop/pause = outage); **network partition** via disconnect/connect (outage); read-only `exec` for internal observation |
| `toxiproxy` (8) | add_latency, packet_loss, bandwidth, blackhole, timeout, partition, clear, reset | latency/bandwidth = degradation; loss/blackhole/timeout/partition = outage; per-stream direction selector; scoped `clear` vs global `reset` |
| `net` (3) | set_dns, nxdomain, dns_blackhole | DNS faults via dnsmasq; nxdomain / blackhole = outage |
| `http` (4) | get (probe), post, put, delete | probe APIs; capture status, latency, JSON body; basic auth; expectStatus |
| `redis` (7) | ping (probe), get (probe), set, del, exists (probe), info (probe), cmd | drive and probe a cache; miss-survival; `info` scrape; arbitrary `cmd` |
| `sql` (3) | query (probe), exec, ping (probe) | **Postgres, MySQL, SQLite**; parameterized; structured rows for assertion |
| `prom` (1) | scrape (probe) | select a metric sample by name + label match, returns float |
| `load` (1) | run | sustained HTTP workload (Vegeta); returns n, errors, errorRate, min/max/mean, p50/p95/p99 |
| `tcp` (1) | connect (probe) | L4 reachability; reports connect latency and success |
| `grpc` (1) | health (probe) | gRPC `health.v1` Health/Check only (plaintext); no arbitrary RPC |
| `exec` (1) | run | the escape hatch; auto-parses JSON stdout |

Faults skew toward outage (10 outage verbs) over degradation (2:
`add_latency`, `bandwidth`); the rest are lifecycle actions or probes.

**Builtins (8 verbs)** **[fact]** (`core/builtins/builtins.go:37-75`): `assert`,
`sleep`, `wait_until`, `background`, `stop_background`, `sample` (window
percentiles), `parallel` (barrier-joined branches, deterministic flush order),
`repeat` (count-based, `stopOnFail`). Assert operators (11): equals, notEquals,
contains, absent, in, matches, gt, lt, gte, lte, between
(`core/builtins/builtins.go:20-23`).

**Composed providers** — domain vocabularies in pure YAML over other verbs
(`jobstore.yml`); the `examples/faults/` project ships kernel-level network
faults (`netem`) and resource-exhaustion (`resource`: cpu/mem/io via Pumba
`stress`) as *composed* providers over `exec` + `background`
(`examples/faults/providers/resource.yml`) **[fact]**. Macro nesting is capped at
1 level (validate rule 4).

**Validation** — 14 static rules, Error vs Warning, run before anything touches a
system; each names file/scenario/step/reason
(`core/validate/validate.go`) **[fact]**. Notably semantic, not just schema:
recovery invariant (rule 7: a captured-work + outage scenario must assert
exactly-once or carry a finding), one-lifecycle-provider (rule 8),
degradation-observed (rule 11), parallel sibling-reference isolation (rule 12),
repeat structure (rule 13), tag syntax (rule 14).

**Verdicts → exit codes** **[fact]** (`core/engine/result.go:18-42`):
PASSED→0, FAILED→1, ERRORED→2, INCONCLUSIVE→3; CLI usage→64. The split is
deliberate: route product regressions (1) to the product team, infra failures
(2/3) to the env owner (`docs/content/how-to/run-in-ci.md:47-48`). Run-level
verdict is worst-of: ERRORED > FAILED > INCONCLUSIVE > PASSED.

**Reports** (written to `shinari-out/`, configurable with `--out`) **[fact]**
(`cli/render/files.go`): console stream with phase column and glyphs,
`results.tsv`, `results.json` (full structure + roll-up verdict + exit code),
`junit.xml` (a finding renders as a *passed* testcase with narrative in
`system-out`), `journal.jsonl` (the raw event stream), `findings.md`
(Injected / Held / Gapped per scenario, with promote-recommendation on a
now-passing finding).

**CLI** **[fact]** (`cli/cmd_*.go`): `new` (scaffold a complete runnable project
from embedded templates), `init` (resolve providers, write checksummed
`shinari.lock.yml`), `validate`, `list`, `explain` (preview a scenario's resolved
timeline without running it), `run`. `run` flags: `--out/-o`, `--dry-run`,
`--keep-up` (env `KEEP_UP=1` is a fallback), `--verbose/-v`,
`--include-tags`/`--exclude-tags` boolean tag expressions (`&`/`|`/`!`/parens,
`core/tagexpr`). Per-project `flock` so a concurrent run fails fast instead of
corrupting state.

**Maturity read [inference].** The *engine* is mature and unusually principled
(deterministic reduction of a result from events, clean seams, semantic
validation, event-gated injection). The *adopter-facing surface* has filled in
notably since earlier drafts: scaffolding (`new`), timeline preview (`explain`),
verbose/keep-up debugging flags, and provider breadth (tcp, grpc-health, redis,
multi-engine sql) all now ship. The remaining thin spots are **longitudinal
memory** (the ledger resets every run), **portable degradation assertions**
(absolute thresholds), **messaging/queue reach**, and **distribution** (sharing
composed providers, editor support, a CI action). That is where adoption friction
now concentrates.

---

## 3. Competitive & solution landscape

The repo cannot tell us what competitors do; the following is **[inference]** from
general knowledge and should be validated before any positioning claim.

| tool | shape | overlap | where Shinari differs |
|---|---|---|---|
| Gremlin / Steadybit | SaaS chaos platforms, agents | fault injection | Shinari is zero-platform, OSS, CI-native, deterministic; they target production / continuous chaos with a UI and a team to operate |
| Chaos Mesh / Litmus | k8s-native chaos operators | fault types | platform-bound to k8s; Shinari runs anywhere a binary runs (k8s is explicitly a future provider, not present today) |
| Toxiproxy | network-fault proxy library | network faults | Shinari *wraps* toxiproxy as one provider; adds lifecycle, gating, assertions, ledger |
| Pumba | docker chaos CLI | process / netem / stress | Shinari *wraps* pumba as composed providers; adds the verdict + ledger model |
| testcontainers | ephemeral infra for tests | bring-up | complementary; Shinari adds fault + assertion + verdict; no native testcontainers bridge today |
| k6 / Vegeta | load generators | workload | Shinari embeds Vegeta as `load`; not a load tool, uses load as a degradation lens |

**Differentiated [inference]:** the findings-ledger-as-living-documentation,
event-gated determinism, and the green-stays-green verdict model. No mainstream
tool combines these in a single binary.
**Table stakes Shinari has:** process + network + DNS faults, network partition,
HTTP/TCP/gRPC-health/Redis/SQL/Prometheus probing, load + degradation
percentiles, JUnit output.
**Behind [inference]:** no historical/trend reporting (the ledger has no memory),
no messaging/queue faults or probes, gRPC limited to health-check, no editor
support, no shareable HTML report, no published CI action, no
continuous/production mode (the last is by design, not a gap).

---

## 4. Personas

1. **Platform / backend engineer (primary).** *Job:* prove "checkout survives a
   cache outage" before it happens, in CI, without standing up a chaos platform.
   *Pain:* chaos tooling assumes k8s plus a team to operate it. *Driver:* one
   binary plus YAML, and `shinari new` now gives a runnable project in seconds.
   *Blocker:* the system under test must be Docker-Compose-shaped to use the
   lifecycle provider, and only one lifecycle provider is allowed (validate
   rule 8).

2. **SRE / on-call owner.** *Job:* keep failure runbooks honest. *Pain:* the wiki
   of "how this fails" decays. *Driver:* the findings ledger is exactly this,
   re-proven each run. *Blocker:* the ledger is **ephemeral per run**; reports
   overwrite `shinari-out/` and there is no "this finding has held for 47 runs /
   this one is new this week" view. The headline value prop has no memory.

3. **QA / integration-test engineer.** *Job:* add resilience cases to an existing
   integration suite. *Driver:* JUnit XML drops straight into existing CI
   dashboards; HTTP/TCP/gRPC-health/SQL/Redis probes now keep the typed
   Observation envelope for most backends. *Blocker:* messaging/queue systems
   (Kafka, RabbitMQ, NATS, SQS) and arbitrary gRPC RPCs still fall back to
   `exec.run`, losing the typed `{value, output, meta}` envelope.

4. **OSS maintainer of a distributed system.** *Job:* ship reproducible failure
   tests as living documentation of guarantees. *Driver:* deterministic event
   gating plus a committed, checksummed lock file equals reproducibility across
   contributors' machines. *Blocker:* no way to share a domain vocabulary
   (composed provider) as a package; everyone re-authors `jobstore.yml`-style
   files, and host dependencies (dnsmasq, pumba) are undeclared.

5. **Eng manager / tech lead (economic buyer of attention).** *Job:* trust a
   green CI signal. *Driver:* the one-red-means-act model. *Blocker:* hard to
   *see* accumulated value without a shareable, visual artifact; `findings.md` is
   plain markdown and resets each run.

---

## 5. Missing core / foundational features (ranked by adoption severity)

**P0 — Longitudinal findings history / baseline.** **[fact + inference,
core-value gap]** The docs state "the document is the product… it cannot go stale
because every line is re-proven." But every run is ephemeral: reports overwrite
`shinari-out/` (`cli/render/files.go`) and nothing persists across runs. There is
no committed ledger, no "what changed since last run" diff, no "this finding is
new this week." The product's headline claim (living documentation of how a
system fails) is undercut by having no memory. A committed/append findings
history plus a since-last-run diff turns the ledger from a per-run snapshot into a
real asset. *This is now the single biggest gap between the stated thesis and the
shipped product. Depends on nothing.*

**P1 — Baseline-relative degradation assertions.** **[fact + inference]**
Degradation checks are hardcoded absolute thresholds today (`p95 gt 250`,
`p99 lt 100` in
`examples/faults/scenarios/network/load-under-latency.yml:34,44`). Absolute
numbers are brittle across machines and CI runners, the very flakiness
event-gating set out to kill, reintroduced at the assertion layer. A recorded
baseline plus relative assertion ("p95 within 20% of baseline", "errorRate delta
< 1 point") would make degradation findings portable and stable. *Composes with
P0: the baseline is the same persisted state.*

**P2 — Messaging/queue reach and richer gRPC.** **[fact]** Probe breadth has
grown a lot (tcp, grpc-health, redis, multi-engine sql, prom), so most request/
response and cache/db backends keep the typed Observation envelope. The remaining
hole is event-driven systems: there is no Kafka/RabbitMQ/NATS/SQS provider, so
queue-depth gating (the docs' own "once the queue depth exceeds 100" example,
`event-gating.md:18`) and message-loss assertions need `exec.run`. gRPC is
health-check only; asserting on an arbitrary unary RPC's response also falls back
to `exec`. A queue probe and a richer gRPC verb would close the typed model for
the asynchronous half of real systems.

**P3 — Distribution of composed providers.** **[fact: absent]** Composed
providers are a genuine differentiator, but there is no way to publish or consume
one: `use:` resolves a local path only, and the lock file checksums local sources
(`examples/quickstart/shinari.lock.yml`). Every team re-authors its domain
vocabulary. A `use:` from a git ref plus a small curated catalog of domain
vocabularies and scenario blueprints would compound the network effect. *The
checksum trust seam already exists; this extends resolution.*

**P4 — Declared host dependencies / preflight.** **[fact]** DNS faults need
dnsmasq; netem and stress need Pumba and `tc`/`iptables` on the host; releases are
Linux/macOS only (no Windows). None of these are checked before a run, so a
first-timer hits an opaque failure mid-scenario. A `shinari doctor` / preflight
that declares and checks the host tools a project's providers require would remove
a class of confusing first-run failures. *Smaller scope, high first-run impact.*

---

## 6. New opportunities (higher-leverage bets)

- **Shareable HTML findings report.** *(low-med effort; independent)* A single
  self-contained `findings.html` (Injected / Held / Gapped, the glyph timeline,
  per-scenario) is the artifact a tech lead pastes into a PR or wiki. The event
  stream and `RunResult` already carry everything needed; this is a renderer, not
  an engine change. Pairs naturally with P0 (history) for a trend view.

- **Editor support: YAML schema + validate-on-save.** *(med effort; independent)*
  Verb arg specs already exist (validate rule 2 checks `with:` against them). Emit
  a JSON Schema / LSP so authors get completion and inline errors in VS Code.
  Lowers the authoring barrier every YAML DSL pays.

- **Official GitHub Action / CI recipes.** *(low effort; independent)* The
  run-in-CI how-to is hand-rolled YAML and no `action.yml` ships. A published
  `shinari-dev/run-action` with caching, artifact upload, and JUnit annotation
  removes copy-paste friction for the CI-native promise.

- **Scenario-level flakiness / stability mode (`run --repeat N` at the scenario
  level).** *(med effort)* A `repeat` builtin exists for steps, but there is no
  way to run a whole scenario N times and report finding stability and degradation
  distribution. This would strengthen the determinism claim by *measuring* it and
  surface genuinely flaky scenarios as a category distinct from findings.

- **Native fault breadth: clock skew, disk pressure.** *(med effort)* Resource
  exhaustion and netem live as composed Pumba wrappers today; clock skew and
  disk-full are common resilience faults with no path at all. Candidates for
  native verbs or first-class shipped composed providers.

- **`testcontainers` / generic-container lifecycle bridge.** *(med effort;
  depends on the rule-8 single-lifecycle constraint)* The lifecycle provider is
  effectively Docker-Compose-only. A generic-container lifecycle provider widens
  "what systems can I test" beyond compose-shaped stacks.

---

## 7. Open questions & risks

- **Adoption signal unknown.** Pre-release, no public users; none of the above is
  validated against real demand. Prioritization is reasoned, not measured.
- **Ledger persistence is a design tension, not just a feature gap.** "Every run
  is ephemeral" is a *correctness* stance (`event-gating.md:60-64`); a persisted
  history (P0) must be additive without reintroducing the drift event-gating
  removed. Needs design care, not just a file. This is the central product
  decision the next phase should resolve.
- **Single lifecycle provider (validate rule 8).** **[fact]** At most one provider
  may implement up/down. How limiting for multi-stack systems (app + broker + db
  across two compose files)? Worth a user test.
- **Composed-provider sharing in practice.** The model is elegant but unproven as
  a distribution mechanism; the "most teams never need more than composed" claim
  (`provider-model.md:41`) needs evidence before investing in P3.
- **Platform reach and host deps.** Linux/macOS only (no Windows); dnsmasq and
  Pumba/`tc` are undeclared host prerequisites that can surprise a first-timer
  (motivates P4). **[fact]**

---

## Top 3 to investigate next

1. **Whether the ledger needs memory (P0).** The headline value prop is "living
   documentation that can't go stale," yet the ledger resets every run. Validate
   with an SRE persona whether per-run findings suffice or whether trend/history
   is the feature that makes the ledger a real asset. This decides whether P0 is a
   feature or the actual product, and it is now the largest gap between the stated
   thesis and the shipped tool.

2. **Non-HTTP backend reach, recounted (P2).** Probe breadth grew (tcp, grpc,
   redis, sql, prom), so re-measure: of the target systems users actually want to
   test, how many still fall to `exec` (messaging/queues, arbitrary gRPC)? If
   async systems are common, the queue provider becomes a P1, not a P2.

3. **First-run success rate.** With `shinari new`, `explain`, and verbose/keep-up
   now shipped, watch a new user go from `curl install` to a first passing
   scenario against a *real* (compose) stack. The likely remaining friction is
   undeclared host dependencies (P4) and the compose-only lifecycle constraint,
   not scaffolding. Measure where the first 15 minutes actually breaks.
