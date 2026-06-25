# Spec: Shinari TUI as a Scenario Workbench

*Status: approved (brainstorm, 2026-06-26). Visual design authored in TUI Studio
(a Figma-like TUI designer at `localhost:5173`); this file is the design of record
for the enhanced control center. Companion to `spec-persistent-ledger-history-tui.md`,
which framed the live/replay TUI at a high level and deferred its concrete design to
here.*

---

## 1. Scope and goal

Grow the existing `shinari tui` control center from a viewer/runner into a
**scenario workbench**, with one cohesive visual language sourced from the docs
site and logo. The loop the TUI supports end to end:

```
browse → scaffold (new) → edit ($EDITOR) → validate → run (with logs) → history
```

**In scope:** a unified bordered-dashboard look on the brand palette; in-TUI editing
via the user's editor; scaffolding a new scenario from a template; a live run view
that exposes the raw event log; the existing scenario list/detail and history tabs
restyled into the same frame.

**Out of scope:** changes to the engine, the event schema, the findings ledger, or
the history store. Those are owned by `spec-persistent-ledger-history-tui.md`. This
spec consumes what the engine already emits (`engine.Event`, `engine.Reduce`,
`RunResult`) and what discovery already records (`model.Scenario.File`). No core
change.

---

## 2. Decisions

| # | Decision | Rationale |
|---|---|---|
| D1 | Redesign the **whole control center** as one visual language, not a single screen. | The user wants cohesion across scenarios, history, and run. |
| D2 | Working loop: **Claude bootstraps `.tui` files, the user art-directs in TUI Studio, Claude hand-writes the Go.** | TUI Studio's BubbleTea export is standalone scaffold that will not wire to the engine event stream, `runModel`, or the detail pane. It is a design surface, not a code generator. |
| D3 | Visual direction: **bordered dashboard.** Rounded-border panels, a persistent top status bar and bottom key bar, focus carried by border color. | Reads as deliberately designed, cohesive, and showcases what TUI Studio does best (boxes and borders). |
| D4 | Edit a scenario by **shelling out to `$EDITOR`** (`tea.ExecProcess` on `scenario.File`), then re-discover and re-validate, showing results inline. | Minimal code, the user's real editor, Unix-idiomatic. `model.Scenario.File` already carries the source path. |
| D5 | "Logs" means the **raw `engine.Event` stream** (time, type, step, key payload fields), shown in a scrollable, toggleable pane in the run view. | Events already carry a `Payload map[string]any`; this is the detail behind each summarized step. No new data needed. |
| D6 | Scaffold a new scenario from a **template** (`minimal` or `fault-inject`), both valid on creation, then drop into `$EDITOR`. | Makes the TUI the authoring entry point; follows the `writing-resilience-scenario` skill. |
| D7 | Palette is **sourced verbatim from `docs/assets/css/main.css` and the logo SVG**, with a fixed dark canvas. | The TUI must match the brand the docs and logo already establish. |

---

## 3. Cohesive visual language

Every screen inherits this system so the app reads as one surface.

- **Outer chrome, always present:**
  - **Top status bar** — rounded-border bar: `shinari` + version on the left, live
    verdict tally on the right (`✓ 3  ✗ 1  ● 1 finding`), project path dimmed.
  - **Tab strip** — `Scenarios │ History`, active tab ember-bold with an accent
    underline, inactive faint.
  - **Bottom key bar** — faint, context-sensitive keys (change per screen and mode).
- **Panels** — rounded borders. The **focused** panel gets an ember border; unfocused
  panels get a dim gray border. This single rule carries focus across every screen.
- **Verdict badges** — short filled chips, not bare words: `PASSED` (green),
  `FAILED`/`ERRORED` (red), `● FINDING` / `NOW PASSES` (ember). Reused identically in
  list rows, detail header, run header, and history.
- **Glyphs** — keep the existing tested set: `✓ ✗ ● –`.

---

## 4. Palette

Sourced from `docs/assets/css/main.css` and `docs/assets/shinari-mark-dark.svg`.
The logo is warm white `#eceae6` (the unbroken line and hexagon) and ember `#ff4f2b`
(the fault dot) on canvas `#0a0b0e`.

| Role | Source var | Hex |
|---|---|---|
| App canvas (fixed) | `--term-bg` | `#0c0e12` |
| Panel fill | `--panel` | `#13151c` |
| Panel fill (soft) | `--panel-soft` | `#181b23` |
| Focused panel border | `--ember` | `#ff4f2b` |
| Unfocused panel border | scrollbar gray | `#2c2f37` |
| Primary text (= logo line) | `--text` | `#eceae6` |
| Soft text | `--text-soft` | `#b4b7bd` |
| Dim text / SKIP | `--text-dim` | `#82868e` |
| Accent (active tab, FINDING, fault dot) | `--ember` | `#ff4f2b` |
| Accent soft | `--ember-soft` | `#ff7a58` |
| PASSED | `--pass` | `#3ecf8e` |
| WARNING (validate) | `--amber` | `#ffb454` |
| Info (log metadata) | `--steel` | `#62b3f0` |
| FAILED / ERRORED | (added) | `#ff5c57` |

**Verdict mapping:** `PASSED` → `#3ecf8e`; `FAILED`/`ERRORED` → `#ff5c57`;
`● FINDING`/`NOW PASSES` → `#ff4f2b`; `⚠ warning` → `#ffb454`; `SKIP` → `#82868e`;
log metadata → `#62b3f0` / dim.

**Two palette decisions:**
- **Fail-red `#ff5c57` is added** because the docs palette has no fail state (the site
  uses ember for all emphasis). A hard red keeps `FAILED` distinct from `FINDING`
  (ember), which the findings-ledger concept depends on.
- **The canvas is fixed dark `#0c0e12`**, matching the docs and logo, rather than
  inheriting the terminal's theme. This replaces today's bare ANSI colors
  (`green "2"`, `red "1"`).

---

## 5. Screens and flows

### 5.1 Scenarios (default)

List + detail, the launch point for edit / new / run.

```
╭─ shinari v0.3 ───────────────────── ✓3 ✗1 ●1 finding ─╮
│  Scenarios │ History                                  │
╰───────────────────────────────────────────────────────╯
╭─ scenarios ──────╮ ╭─ cache-outage ········· PASSED ──╮
│ ▸ cache-outage ✓ │ │ Overview · Explain · Dry-run     │
│   db-failover  ● │ │ kills redis, asserts recovery<5s │
│   net-partition ✓│ │ setup 2 · method 1 · verify 3    │
╰──────────────────╯ ╰──────────────────────────────────╯
 tab  ↵ edit  n new  r run  e explain  d dry-run  q quit
```

List rows gain a trailing verdict glyph (last-known result per scenario). Detail
header carries the badge; the Overview / Explain / Dry-run sub-tabs are unchanged.

### 5.2 Edit ($EDITOR) and validate

`↵` on a selected scenario suspends the TUI and opens `scenario.File` in `$EDITOR`
via `tea.ExecProcess`. On return the project is re-discovered and the edited scenario
re-validated; results are shown inline.

```
╭─ validate · cache-outage ─────────────╮
│ ✓ 0 errors   ⚠ 1 warning              │
│  ⚠ finding has no explicit id (derived)│
╰────────────────────────────────────────╯
```

### 5.3 New (scaffold)

`n` opens a small modal. On confirm it writes `scenarios/<suite>/<name>.yml` from the
chosen template, drops into `$EDITOR`, re-discovers, and selects the new scenario.

```
╭─ new scenario ───────────────────╮
│ name   ▏cache-outage             │
│ suite  ▏resilience               │
│ template ◉ minimal ○ fault-inject│
│ ↵ create   esc cancel            │
╰──────────────────────────────────╯
```

Templates are valid on creation so validate is green immediately. `minimal` is a
bare lifecycle skeleton; `fault-inject` adds a `method` step with an `effect` and a
`finding`, per the `writing-resilience-scenario` skill.

### 5.4 Run with logs

The run panel splits: step summary on top, a scrolling raw **event log** below,
rendered from each `Event`'s type and payload. `l` cycles the log pane size
(collapsed / half / full); `↑↓` scroll it. On finish the header shows the verdict and
`recorded ✓`.

```
╭─ run · cache-outage ···················· RUNNING ─╮
│  ✓ steady state                                   │
│  ● method: kill redis              inject         │
│  ┄ recovery (gate)…                               │
├─ logs ────────────────────────────── l collapse ─┤
│ 12:03:01 step.started   redis.kill                │
│ 12:03:01 fault.injected redis.kill  outage        │
│ 12:03:06 step.failed    recovery 6.2s > 5s        │
╰───────────────────────────────────────────────────╯
 x cancel  l logs  q quit
```

### 5.5 History

The per-finding trend table, restyled into one bordered panel, with status badges and
a compact `●●●○` per-finding run strip (filled = present that run, hollow = absent).

```
╭─ findings ledger ── 12 runs recorded ────────────────╮
│  cache.recovery.slow   ●●●○  9 runs   open    p99…   │
│  db.failover.dataloss  ○○●●  4 runs   fixed   row…   │
╰──────────────────────────────────────────────────────╯
```

---

## 6. Keymap

No rebinds of existing keys are required.

| Key | Context | Action |
|---|---|---|
| `↵` | scenario selected | edit in `$EDITOR` (already means "back" only in the run-done mode, a different context) |
| `n` | scenarios tab | new scenario (scaffold) |
| `l` | run view | cycle log pane size |
| `↑↓ j k` | list / log | move / scroll |
| `o` `e` `d` | detail | Overview / Explain / Dry-run |
| `r` | scenarios tab | run selected |
| `tab` | any | switch tab |
| `/` | list | filter |
| `x` | run (running) | cancel |
| `q` | any | quit |

---

## 7. `.tui` bootstrap deliverables

Authored by Claude into `design/tui/`, opened and art-directed by the user in TUI
Studio, saved back, then used as the reference Claude implements against:

- `scenarios.tui` — full frame: status bar, tab strip, list panel, detail panel, key bar.
- `run-with-logs.tui` — run panel with the steps/log split.
- `history.tui` — findings-ledger panel.
- `new-scenario.tui` — the scaffold modal.

Each is a complete ember-themed frame using the §4 hex values (TUI Studio has no
"ember" preset theme, so colors are set explicitly per node).

---

## 8. Implementation notes (fitting existing patterns)

The TUI stays at the CLI edge; core is untouched. Concrete touch points:

- **Styling.** Replace the ad-hoc `lipgloss` styles in `render.go`, `app.go`,
  `detail.go` with one shared palette/style set (the §4 colors). Introduce verdict-badge
  and panel-border helpers reused across screens.
- **Chrome.** `App.View` gains the status bar (verdict tally folded from the last run /
  history) and a context key bar; panels render with the focus-border rule.
- **Edit + validate.** `App.Update` handles `↵` by returning a `tea.ExecProcess`
  command on `scenario.File`; on completion it re-runs discovery and the existing
  `validate` pass and stores the result for an inline panel. Reuse the CLI's validate
  renderer where possible.
- **Scaffold.** A small template writer (templates `minimal`, `fault-inject`) writes
  under `scenarios/<suite>/<name>.yml`, then the same `tea.ExecProcess` edit path runs,
  then re-discover. Keep the templates beside the TUI or in a small `cli` helper.
- **Logs.** A new render function over `[]engine.Event` (verbose: time, type, step,
  payload), shown in a `viewport` inside `runModel`; `l` cycles its height.
- **List verdict glyphs.** The list delegate shows a trailing glyph from the
  last-known verdict per scenario (from history records already available to `App`).

Existing tests in `cli/tui/*_test.go` constrain the refactor; rendering stays pure
(`RenderRun` and the new log renderer take data in, return strings out).

---

## 9. Build order

1. Shared palette and style helpers (§4): badges, panel borders, glyphs.
2. Outer chrome: status bar + tab strip + context key bar.
3. Scenarios screen restyle (list glyphs, detail panel, focus border).
4. Run-with-logs: the event-log renderer and the split/toggle.
5. Edit via `$EDITOR` + re-discover + inline validate panel.
6. Scaffold: templates + writer + edit path + re-discover.
7. History restyle with the run-strip.

---

## 10. Open questions

- **Status-bar tally source.** Fold from the most recent run in the session, or from
  the latest history record per scenario? Default to the session's last run, fall back
  to history. Confirm during implementation.
- **Log payload rendering.** Which payload keys are worth surfacing per event type
  (command, output, error, latency)? Decide against real `Payload` contents from
  `executor.go` when wiring the log renderer.
