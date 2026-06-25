# TUI Scenario Workbench Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Grow `shinari tui` from a viewer/runner into a scenario workbench (browse → scaffold → edit → validate → run-with-logs → history) on the brand palette, with the logo present.

**Architecture:** All work is at the CLI edge in `cli/tui/` (plus a few injection lines in `cli/cmd_tui.go`); core is untouched. Rendering stays pure (data in → string out) so it is unit-testable, mirroring the existing `RenderRun`/`render_test.go` pattern. Edit/scaffold shell out via `tea.ExecProcess` and re-run `discover.Load` + `validate.Validate` in-process.

**Tech Stack:** Go, Bubble Tea v1.3.10, Bubbles (list/viewport/textinput/key/help), Lipgloss.

## Global Constraints

- SPDX header on every new file: `// SPDX-FileCopyrightText: 2026 The Shinari Authors` then `// SPDX-License-Identifier: Apache-2.0`.
- `cli/tui` may import `core/...` and `cli/history` only (never a concrete provider). Core stays untouched.
- Palette (hex, verbatim): canvas `#0c0e12`, panel `#13151c`, panel-soft `#181b23`, ember `#ff4f2b`, ember-soft `#ff7a58`, border-dim `#2c2f37`, text `#eceae6`, text-soft `#b4b7bd`, text-dim `#82868e`, pass `#3ecf8e`, warn `#ffb454`, steel `#62b3f0`, fail `#ff5c57`.
- Tests are hermetic and offline. Run `go test ./cli/tui/` per task; `go build -o shinari ./cli`, `go vet ./...`, `go test ./...` at the end.
- Keep files focused: new responsibilities go in new files (`theme.go`, `chrome.go`, `logview.go`, `scaffold.go`, `logo.go`), not piled into `app.go`.

---

### Task 1: Visual foundation — palette, badges, glyphs, chrome, keymap

**Files:**
- Create: `cli/tui/theme.go`
- Create: `cli/tui/chrome.go`
- Modify: `cli/tui/keys.go` (add `Edit`, `New`, `Logs` bindings)
- Test: `cli/tui/theme_test.go`, `cli/tui/chrome_test.go`

**Interfaces:**
- Produces:
  - `var (canvas, panel, panelSoft, ember, emberSoft, borderDim, fg, fgSoft, fgDim, pass, warn, steel, fail lipgloss.Color)` — palette.
  - `func badge(label string, fgc, bgc lipgloss.Color) string` — filled chip ` label `.
  - `func verdictBadge(v string) string` — maps a verdict/finding string to a colored badge.
  - `func glyph(v engine.CheckVerdict) string` — the `✓ ✗ ● –` set (moves the body of `stepGlyph`).
  - `func panelStyle(title string, w, h int, focused bool) lipgloss.Style` — rounded border (ember if focused else border-dim) sized `w×h`.
  - `func renderStatusBar(tally tallyCounts, width int) string` — bordered bar: `● shinari v0.3` left, tally right.
  - `func renderTabs(active string) string`, `func renderKeyBar(keys []key.Binding, h help.Model) string`.
  - `type tallyCounts struct{ Pass, Fail, Findings int }`.

- [ ] **Step 1: Write the failing tests**

```go
// cli/tui/theme_test.go
package tui

import (
	"strings"
	"testing"

	"github.com/shinari-dev/shinari/core/engine"
)

func TestVerdictBadgeContainsLabel(t *testing.T) {
	if !strings.Contains(verdictBadge("PASSED"), "PASSED") {
		t.Fatal("badge must contain its label")
	}
}

func TestGlyphSet(t *testing.T) {
	cases := map[engine.CheckVerdict]string{
		engine.CheckPass: "✓", engine.CheckFail: "✗", engine.CheckFinding: "●", engine.CheckSkip: "–",
	}
	for v, want := range cases {
		if !strings.Contains(glyph(v), want) {
			t.Fatalf("glyph(%v) = %q, want %q", v, glyph(v), want)
		}
	}
}
```

```go
// cli/tui/chrome_test.go
package tui

import (
	"strings"
	"testing"
)

func TestStatusBarHasBrandmarkAndTally(t *testing.T) {
	out := renderStatusBar(tallyCounts{Pass: 3, Fail: 1, Findings: 1}, 80)
	for _, want := range []string{"●", "shinari", "3", "1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status bar missing %q in:\n%s", want, out)
		}
	}
}

func TestTabsMarksActive(t *testing.T) {
	if !strings.Contains(renderTabs("History"), "History") || !strings.Contains(renderTabs("History"), "Scenarios") {
		t.Fatal("tabs must list both tabs")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cli/tui/ -run 'TestVerdictBadge|TestGlyphSet|TestStatusBar|TestTabs' -v`
Expected: FAIL — undefined: `verdictBadge`, `glyph`, `renderStatusBar`, `renderTabs`, `tallyCounts`.

- [ ] **Step 3: Implement `theme.go`**

```go
// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/shinari-dev/shinari/core/engine"
)

// Palette — sourced from docs/assets/css/main.css + the logo SVG.
var (
	canvas    = lipgloss.Color("#0c0e12")
	panel     = lipgloss.Color("#13151c")
	panelSoft = lipgloss.Color("#181b23")
	ember     = lipgloss.Color("#ff4f2b")
	emberSoft = lipgloss.Color("#ff7a58")
	borderDim = lipgloss.Color("#2c2f37")
	fg        = lipgloss.Color("#eceae6")
	fgSoft    = lipgloss.Color("#b4b7bd")
	fgDim     = lipgloss.Color("#82868e")
	pass      = lipgloss.Color("#3ecf8e")
	warn      = lipgloss.Color("#ffb454")
	steel     = lipgloss.Color("#62b3f0")
	fail      = lipgloss.Color("#ff5c57")
)

func badge(label string, fgc, bgc lipgloss.Color) string {
	return lipgloss.NewStyle().Foreground(fgc).Background(bgc).Bold(true).Render(" " + label + " ")
}

// verdictBadge maps a scenario/finding verdict string to a colored chip.
func verdictBadge(v string) string {
	switch v {
	case string(engine.ScenarioPassed):
		return badge(v, canvas, pass)
	case string(engine.ScenarioFailed), string(engine.ScenarioErrored):
		return badge(v, canvas, fail)
	case "FINDING", "NOW PASSES", "RUNNING":
		return badge(v, canvas, ember)
	default:
		return lipgloss.NewStyle().Foreground(fgSoft).Render(v)
	}
}

func glyph(v engine.CheckVerdict) string {
	switch v {
	case engine.CheckPass:
		return lipgloss.NewStyle().Foreground(pass).Render("✓")
	case engine.CheckFail:
		return lipgloss.NewStyle().Foreground(fail).Bold(true).Render("✗")
	case engine.CheckFinding:
		return lipgloss.NewStyle().Foreground(ember).Render("●")
	case engine.CheckSkip:
		return lipgloss.NewStyle().Foreground(fgDim).Render("–")
	default:
		return " "
	}
}

// panelStyle is a rounded border sized w×h; ember when focused, dim otherwise.
func panelStyle(w, h int, focused bool) lipgloss.Style {
	bc := borderDim
	if focused {
		bc = ember
	}
	s := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(bc).Foreground(fg)
	if w > 2 {
		s = s.Width(w - 2)
	}
	if h > 2 {
		s = s.Height(h - 2)
	}
	return s
}
```

- [ ] **Step 4: Implement `chrome.go`**

```go
// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

type tallyCounts struct{ Pass, Fail, Findings int }

func renderStatusBar(t tallyCounts, width int) string {
	dot := lipgloss.NewStyle().Foreground(ember).Bold(true).Render("●")
	word := lipgloss.NewStyle().Foreground(fg).Bold(true).Render("shinari")
	ver := lipgloss.NewStyle().Foreground(fgDim).Render("v0.3")
	left := dot + " " + word + " " + ver
	tally := fmt.Sprintf("%s %d   %s %d   %s %d finding",
		lipgloss.NewStyle().Foreground(pass).Render("✓"), t.Pass,
		lipgloss.NewStyle().Foreground(fail).Render("✗"), t.Fail,
		lipgloss.NewStyle().Foreground(ember).Render("●"), t.Findings)
	gap := width - lipgloss.Width(left) - lipgloss.Width(tally) - 4
	if gap < 1 {
		gap = 1
	}
	row := left + lipgloss.NewStyle().Width(gap).Render("") + tally
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderDim).
		Width(width - 2).Render(row)
}

func renderTabs(active string) string {
	on := lipgloss.NewStyle().Foreground(ember).Bold(true).Underline(true)
	off := lipgloss.NewStyle().Foreground(fgDim)
	mark := func(s string) string {
		if s == active {
			return on.Render(s)
		}
		return off.Render(s)
	}
	sep := lipgloss.NewStyle().Foreground(borderDim).Render("  │  ")
	return mark("Scenarios") + sep + mark("History")
}

func renderKeyBar(keys []key.Binding, h help.Model) string {
	return h.ShortHelpView(keys)
}
```

- [ ] **Step 5: Add keymap bindings in `keys.go`**

Add to the `keyMap` struct and `defaultKeys()`:

```go
// in struct keyMap, after Run:
	Edit    key.Binding
	New     key.Binding
	Logs    key.Binding
```
```go
// in defaultKeys(), inside the returned keyMap literal:
		Edit:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "edit")),
		New:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		Logs:    key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs")),
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./cli/tui/ -run 'TestVerdictBadge|TestGlyphSet|TestStatusBar|TestTabs' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cli/tui/theme.go cli/tui/chrome.go cli/tui/theme_test.go cli/tui/chrome_test.go cli/tui/keys.go
git commit -m "feat(tui): brand palette, badges, chrome, workbench keymap"
```

---

### Task 2: Event-log renderer

**Files:**
- Create: `cli/tui/logview.go`
- Test: `cli/tui/logview_test.go`

**Interfaces:**
- Consumes: `engine.Event` (fields `Type`, `Time`, `Step`, `Verb`, `Payload map[string]any`).
- Produces: `func RenderLog(events []engine.Event) []string` — one line per event: `HH:MM:SS  type  step/verb  key=val…`, colored (steel time, dim type, fail on `verdict=FAILED`/`error`). Returns plain `[]string` so callers control height/scrolling.

- [ ] **Step 1: Write the failing test**

```go
// cli/tui/logview_test.go
package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/shinari-dev/shinari/core/engine"
)

func TestRenderLogLinePerEvent(t *testing.T) {
	evs := []engine.Event{
		{Type: engine.EvStepStarted, Time: time.Unix(0, 0).UTC(), Step: "kill redis", Verb: "redis.kill"},
		{Type: engine.EvStepFailed, Time: time.Unix(6, 0).UTC(), Step: "recovery", Payload: map[string]any{"verdict": "FAILED", "error": "6.2s > 5s"}},
	}
	lines := RenderLog(evs)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "step.started") || !strings.Contains(lines[0], "redis.kill") {
		t.Fatalf("line 0 missing type/verb: %q", lines[0])
	}
	if !strings.Contains(lines[1], "6.2s > 5s") {
		t.Fatalf("line 1 missing payload detail: %q", lines[1])
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cli/tui/ -run TestRenderLog -v`
Expected: FAIL — undefined: `RenderLog`.

- [ ] **Step 3: Implement `logview.go`**

```go
// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/shinari-dev/shinari/core/engine"
)

// payloadKeys surfaced in the log, in priority order; the rest are dropped.
var logKeys = []string{"verdict", "error", "reason", "value", "observed", "narrative", "detail"}

// RenderLog turns the raw event stream into one display line per event.
func RenderLog(events []engine.Event) []string {
	timeS := lipgloss.NewStyle().Foreground(steel)
	typeS := lipgloss.NewStyle().Foreground(fgDim)
	textS := lipgloss.NewStyle().Foreground(fgSoft)
	failS := lipgloss.NewStyle().Foreground(fail)

	lines := make([]string, 0, len(events))
	for _, e := range events {
		target := e.Step
		if target == "" {
			target = e.Verb
		}
		detail := logDetail(e.Payload)
		style := textS
		if e.Type == engine.EvStepFailed {
			style = failS
		}
		lines = append(lines,
			timeS.Render(e.Time.Format("15:04:05"))+"  "+
				typeS.Render(string(e.Type))+"  "+
				style.Render(strings.TrimSpace(target+"  "+detail)))
	}
	return lines
}

func logDetail(p map[string]any) string {
	if p == nil {
		return ""
	}
	var parts []string
	for _, k := range logKeys { // fixed order; logKeys is the priority list
		if v, ok := p[k]; ok && v != nil && v != "" {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	return strings.Join(parts, " ")
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cli/tui/ -run TestRenderLog -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cli/tui/logview.go cli/tui/logview_test.go
git commit -m "feat(tui): raw event-log renderer"
```

---

### Task 3: Run view — logs split with size toggle

**Files:**
- Modify: `cli/tui/runmodel.go`
- Test: `cli/tui/runmodel_test.go` (add cases)

**Interfaces:**
- Consumes: `RenderLog` (Task 2), `engine.Reduce`, `RenderRun`.
- Produces: `runModel` gains `logSize int` (0=collapsed,1=half,2=full) and `cycleLogs()`; `Update` handles the `Logs` key; `View` joins the step summary and a logs panel.

- [ ] **Step 1: Write the failing test**

```go
// add to cli/tui/runmodel_test.go
func TestRunModelCyclesLogSize(t *testing.T) {
	r := newRun()
	if r.logSize != 1 {
		t.Fatalf("default logSize want 1, got %d", r.logSize)
	}
	r = r.cycleLogs()
	if r.logSize != 2 {
		t.Fatalf("after cycle want 2, got %d", r.logSize)
	}
	r = r.cycleLogs()
	if r.logSize != 0 {
		t.Fatalf("wrap want 0, got %d", r.logSize)
	}
}

func TestRunModelViewShowsLogLines(t *testing.T) {
	r := newRun()
	r.events = []engine.Event{{Type: engine.EvStepStarted, Step: "kill redis", Verb: "redis.kill"}}
	if !strings.Contains(r.View(80), "redis.kill") {
		t.Fatalf("run view should include log content")
	}
}
```

Add imports `strings` and (already present) `engine` to the test file if missing.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cli/tui/ -run TestRunModel -v`
Expected: FAIL — `r.logSize`/`r.cycleLogs` undefined.

- [ ] **Step 3: Implement in `runmodel.go`**

Add `logSize int` to the `runModel` struct, set it in `newRun`, add `cycleLogs`, handle the key in `Update`, and render the split in `View`:

```go
func newRun() runModel { return runModel{logSize: 1} }

func (r runModel) cycleLogs() runModel {
	r.logSize = (r.logSize + 1) % 3
	return r
}
```
```go
// in Update's switch, add a KeyMsg case BEFORE the default return:
	case tea.KeyMsg:
		if msg.String() == "l" {
			return r.cycleLogs(), nil
		}
		return r, nil
```
```go
// replace View with the split version:
func (r runModel) View(width int) string {
	header := verdictBadge("RUNNING")
	if r.done {
		header = verdictBadge(string(r.res.Verdict()))
		if r.afterRan {
			header += lipgloss.NewStyle().Foreground(fgDim).Render("  ·  recorded ✓")
		}
	}
	summary := RenderRun(engine.Reduce(r.events), "run · "+r.scenarioName(), width)
	body := lipgloss.JoinVertical(lipgloss.Left, header, summary)
	if r.logSize > 0 {
		n := 6
		if r.logSize == 2 {
			n = 16
		}
		logs := RenderLog(r.events)
		if len(logs) > n {
			logs = logs[len(logs)-n:]
		}
		title := lipgloss.NewStyle().Foreground(fgDim).Render("logs  (l)")
		box := lipgloss.JoinVertical(lipgloss.Left, append([]string{title}, logs...)...)
		body = lipgloss.JoinVertical(lipgloss.Left, body, box)
	}
	return body
}

func (r runModel) scenarioName() string {
	if r.done {
		if len(r.res.Scenarios) > 0 {
			return r.res.Scenarios[0].Name
		}
	}
	red := engine.Reduce(r.events)
	if len(red.Scenarios) > 0 {
		return red.Scenarios[0].Name
	}
	return ""
}
```

Note: `RenderRun`'s second arg is the header string; the run header now lives above it, so pass the scenario title. Keep the existing `cancel`/`AfterRun`/`DoneMsg` logic intact.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./cli/tui/ -run TestRunModel -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cli/tui/runmodel.go cli/tui/runmodel_test.go
git commit -m "feat(tui): run view logs split with size toggle"
```

---

### Task 4: Scaffold — templates and writer

**Files:**
- Create: `cli/tui/scaffold.go`
- Test: `cli/tui/scaffold_test.go`

**Interfaces:**
- Produces:
  - `func scenarioTemplate(name, kind string) string` — valid scenario YAML; `kind` is `"minimal"` or `"fault-inject"`.
  - `func writeScenario(root, suite, name, kind string) (string, error)` — writes `root/scenarios/<suite>/<name>.yml`, returns the path; errors if the file exists.

- [ ] **Step 1: Write the failing test**

```go
// cli/tui/scaffold_test.go
package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/core/validate"
)

func TestScenarioTemplateParsesAndValidates(t *testing.T) {
	for _, kind := range []string{"minimal", "fault-inject"} {
		yml := scenarioTemplate("cache-outage", kind)
		sc, err := model.ParseScenario([]byte(yml), "cache-outage.yml")
		if err != nil {
			t.Fatalf("%s template did not parse: %v", kind, err)
		}
		if sc.Name == "" {
			t.Fatalf("%s template missing name", kind)
		}
	}
}

func TestWriteScenarioCreatesValidProject(t *testing.T) {
	root := t.TempDir()
	path, err := writeScenario(root, "resilience", "cache-outage", "minimal")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if filepath.Base(path) != "cache-outage.yml" {
		t.Fatalf("unexpected path %s", path)
	}
	set, err := discover.Load(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	for _, f := range validate.Validate(set) {
		if f.Severity == validate.Error {
			t.Fatalf("scaffolded scenario has a validate error: %s", f.String())
		}
	}
	if _, err := writeScenario(root, "resilience", "cache-outage", "minimal"); err == nil {
		t.Fatal("writing over an existing file should error")
	}
	_ = os.Remove(path)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cli/tui/ -run 'TestScenarioTemplate|TestWriteScenario' -v`
Expected: FAIL — undefined: `scenarioTemplate`, `writeScenario`.

- [ ] **Step 3: Implement `scaffold.go`**

```go
// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"
	"os"
	"path/filepath"
)

func scenarioTemplate(name, kind string) string {
	head := fmt.Sprintf("apiVersion: shinari/v1\nkind: Scenario\nname: %s\ndescription: TODO describe what resilience property this asserts.\n\n", name)
	minimal := head +
		"setup:\n  - run: exec.run\n    with: \"true\"\n\n" +
		"steadyState:\n  - run: assert\n    with: { of: \"${true}\", eq: true }\n    desc: \"system is up\"\n\n" +
		"method:\n  - phase: \"Inject a fault\"\n    steps:\n      - run: assert\n        with: { of: \"${true}\", eq: true }\n        desc: \"placeholder method step\"\n\n" +
		"verify:\n  - run: assert\n    with: { of: \"${true}\", eq: true }\n    desc: \"system recovered\"\n\n" +
		"teardown:\n  - run: exec.run\n    with: \"true\"\n"
	if kind == "minimal" {
		return minimal
	}
	return head +
		"setup:\n  - run: exec.run\n    with: \"true\"\n\n" +
		"steadyState:\n  - run: assert\n    with: { of: \"${true}\", eq: true }\n    desc: \"system is up\"\n\n" +
		"method:\n  - phase: \"Inject a fault and observe recovery\"\n    steps:\n      - run: exec.run\n        with: \"true\"\n        effect: outage\n        desc: \"inject the fault here (replace with a real fault verb)\"\n      - run: assert\n        with: { of: \"${true}\", eq: true }\n        finding: \"recovery is slower than the target\"\n\n" +
		"verify:\n  - run: assert\n    with: { of: \"${true}\", eq: true }\n    desc: \"system recovered\"\n\n" +
		"teardown:\n  - run: exec.run\n    with: \"true\"\n"
}

// writeScenario writes a scaffolded scenario under scenarios/<suite>/<name>.yml.
func writeScenario(root, suite, name, kind string) (string, error) {
	dir := filepath.Join(root, "scenarios", suite)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".yml")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists", path)
	}
	if err := os.WriteFile(path, []byte(scenarioTemplate(name, kind)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
```

Note: verify the `assert`/`exec.run` shapes against the `writing-resilience-scenario` skill while implementing; if `exec.run with: "true"` is not the right shorthand, adjust the template so `TestScenarioTemplateParsesAndValidates` and the discover+validate test both pass. The test is the contract: the template must parse and produce zero validate Errors.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./cli/tui/ -run 'TestScenarioTemplate|TestWriteScenario' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cli/tui/scaffold.go cli/tui/scaffold_test.go
git commit -m "feat(tui): scenario scaffold templates and writer"
```

---

### Task 5: Edit via $EDITOR, re-discover, inline validate panel

**Files:**
- Modify: `cli/tui/app.go`
- Modify: `cli/cmd_tui.go` (inject `Editor`)
- Test: `cli/tui/app_test.go` (add cases)

**Interfaces:**
- Consumes: `discover.Load`, `validate.Validate`, `tea.ExecProcess`, the `Edit` key (Task 1).
- Produces: `App` gains `Editor string`, `validateMsg`/`reDiscoverMsg` handling, `validateSummary string`; `App.editSelected() (tea.Model, tea.Cmd)`; status-tally computed from `History`.

- [ ] **Step 1: Write the failing test**

```go
// add to cli/tui/app_test.go
// appFromTempProject scaffolds one scenario and builds an App over it.
func appFromTempProject(t *testing.T) App {
	t.Helper()
	root := t.TempDir()
	if _, err := writeScenario(root, "s", "demo", "minimal"); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	set, err := discover.Load(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	return NewApp(set)
}

func TestReDiscoverUpdatesValidateSummary(t *testing.T) {
	a := appFromTempProject(t)
	na, _ := a.Update(reDiscoverMsg{})
	if na.(App).validateSummary == "" {
		t.Fatal("re-discover should populate a validate summary")
	}
}
```

Add `"github.com/shinari-dev/shinari/core/discover"` to the test imports if not already present.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cli/tui/ -run TestReDiscover -v`
Expected: FAIL — `reDiscoverMsg`/`validateSummary` undefined.

- [ ] **Step 3: Implement in `app.go`**

Add fields and message types, handle them in `Update`, and add the edit command:

```go
// new message types (near EventMsg/DoneMsg or in app.go):
type reDiscoverMsg struct{}
type editDoneMsg struct{ err error }
```
```go
// App struct: add
	Editor          string
	validateSummary string
```
```go
// in Update, add cases:
	case reDiscoverMsg:
		if ns, err := discover.Load(a.set.Root); err == nil {
			a.set = ns
			items := make([]list.Item, 0, len(ns.Scenarios))
			for _, sc := range ns.Scenarios {
				items = append(items, scenarioItem{sc: sc})
			}
			a.list.SetItems(items)
			a.validateSummary = summarizeValidate(validate.Validate(ns))
		}
		return a, nil
	case editDoneMsg:
		return a, func() tea.Msg { return reDiscoverMsg{} }
```
```go
// in the tea.KeyMsg branch, in the tabScenarios section, before the o/e/d switch:
	if a.tab == tabScenarios && key.Matches(msg, a.keys.Edit) && a.list.FilterState() != list.Filtering {
		return a.editSelected()
	}
```
```go
func (a App) editSelected() (tea.Model, tea.Cmd) {
	sc := a.Selected()
	if sc == nil || sc.File == "" || a.Editor == "" {
		return a, nil
	}
	c := exec.Command(a.Editor, sc.File) //nolint:gosec // user's own editor on their own file
	return a, tea.ExecProcess(c, func(err error) tea.Msg { return editDoneMsg{err} })
}

func summarizeValidate(fs []validate.Finding) string {
	errs, warns := 0, 0
	for _, f := range fs {
		if f.Severity == validate.Error {
			errs++
		} else {
			warns++
		}
	}
	ok := lipgloss.NewStyle().Foreground(pass).Render("✓")
	w := lipgloss.NewStyle().Foreground(warn).Render("⚠")
	return fmt.Sprintf("%s %d errors   %s %d warnings", ok, errs, w, warns)
}
```

Add imports to `app.go`: `os/exec`, `fmt`, `github.com/charmbracelet/lipgloss`, `github.com/shinari-dev/shinari/core/validate`. Render `a.validateSummary` in `View` under the detail panel when non-empty.

- [ ] **Step 4: Inject `Editor` in `cmd_tui.go`**

After `app := tui.NewApp(set)`:
```go
	app.Editor = getenv("EDITOR")
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./cli/tui/ -run TestReDiscover -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cli/tui/app.go cli/cmd_tui.go cli/tui/app_test.go
git commit -m "feat(tui): edit scenario in \$EDITOR, re-discover and validate"
```

---

### Task 6: New-scenario scaffold flow (modal form)

**Files:**
- Create: `cli/tui/newscenario.go`
- Modify: `cli/tui/app.go` (open/route the form; on submit call `writeScenario` then edit path)
- Test: `cli/tui/newscenario_test.go`

**Interfaces:**
- Consumes: `writeScenario` (Task 4), the `New` key, `editDoneMsg`/`reDiscoverMsg` (Task 5), `bubbles/textinput`.
- Produces: `type newModel struct{…}`, `func newNewModel() newModel`, `func (newModel) Update(tea.Msg) (newModel, tea.Cmd)`, `func (newModel) View() string`, `func (newModel) submit(root string) tea.Cmd` returning a cmd that writes the file and emits `createdMsg{path string, err error}`; `App` gains `newForm *newModel`.

- [ ] **Step 1: Write the failing test**

```go
// cli/tui/newscenario_test.go
package tui

import (
	"strings"
	"testing"
)

func TestNewModelViewHasFields(t *testing.T) {
	m := newNewModel()
	out := m.View()
	for _, want := range []string{"name", "suite", "minimal", "fault-inject"} {
		if !strings.Contains(out, want) {
			t.Fatalf("new-scenario form missing %q in:\n%s", want, out)
		}
	}
}

func TestNewModelSubmitWritesFile(t *testing.T) {
	root := t.TempDir()
	m := newNewModel()
	m.name.SetValue("demo")
	m.suite.SetValue("s")
	cmd := m.submit(root)
	msg := cmd()
	cm, ok := msg.(createdMsg)
	if !ok || cm.err != nil {
		t.Fatalf("submit failed: %#v", msg)
	}
	if !strings.HasSuffix(cm.path, "s/demo.yml") {
		t.Fatalf("unexpected path %s", cm.path)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cli/tui/ -run TestNewModel -v`
Expected: FAIL — undefined `newNewModel`, `createdMsg`.

- [ ] **Step 3: Implement `newscenario.go`**

```go
// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type createdMsg struct {
	path string
	err  error
}

type newModel struct {
	name    textinput.Model
	suite   textinput.Model
	kind    string // "minimal" | "fault-inject"
	focus   int    // 0 name, 1 suite
}

func newNewModel() newModel {
	n := textinput.New()
	n.Placeholder = "cache-outage"
	n.Focus()
	s := textinput.New()
	s.Placeholder = "resilience"
	s.SetValue("resilience")
	return newModel{name: n, suite: s, kind: "minimal"}
}

func (m newModel) Update(msg tea.Msg) (newModel, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "tab":
			m.focus = (m.focus + 1) % 2
			if m.focus == 0 {
				m.name.Focus()
				m.suite.Blur()
			} else {
				m.suite.Focus()
				m.name.Blur()
			}
			return m, nil
		case "t":
			if m.kind == "minimal" {
				m.kind = "fault-inject"
			} else {
				m.kind = "minimal"
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	if m.focus == 0 {
		m.name, cmd = m.name.Update(msg)
	} else {
		m.suite, cmd = m.suite.Update(msg)
	}
	return m, cmd
}

func (m newModel) submit(root string) tea.Cmd {
	name, suite, kind := m.name.Value(), m.suite.Value(), m.kind
	return func() tea.Msg {
		path, err := writeScenario(root, suite, name, kind)
		return createdMsg{path: path, err: err}
	}
}

func (m newModel) View() string {
	sel := func(k string) string {
		if k == m.kind {
			return lipgloss.NewStyle().Foreground(ember).Render("◉ " + k)
		}
		return lipgloss.NewStyle().Foreground(fgDim).Render("○ " + k)
	}
	rows := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Foreground(ember).Bold(true).Render("new scenario"),
		"name   "+m.name.View(),
		"suite  "+m.suite.View(),
		"template  "+sel("minimal")+"  "+sel("fault-inject")+lipgloss.NewStyle().Foreground(fgDim).Render("  (t)"),
		lipgloss.NewStyle().Foreground(fgDim).Render("↵ create   esc cancel"),
	)
	return panelStyle(50, 9, true).Render(rows)
}
```

- [ ] **Step 4: Wire into `app.go`**

```go
// App struct: add
	newForm *newModel
```
```go
// in the tea.KeyMsg branch, before the run/tab handling, route the form when open:
	if a.newForm != nil {
		switch {
		case key.Matches(msg, a.keys.Back):
			a.newForm = nil
			return a, nil
		case msg.String() == "enter":
			cmd := a.newForm.submit(a.set.Root)
			a.newForm = nil
			return a, cmd
		}
		nf, cmd := a.newForm.Update(msg)
		a.newForm = &nf
		return a, cmd
	}
	if a.tab == tabScenarios && key.Matches(msg, a.keys.New) && a.list.FilterState() != list.Filtering {
		nf := newNewModel()
		a.newForm = &nf
		return a, nil
	}
```
```go
// add a case in Update for the created file -> open editor then re-discover:
	case createdMsg:
		if msg.err != nil {
			a.validateSummary = lipgloss.NewStyle().Foreground(fail).Render(msg.err.Error())
			return a, nil
		}
		if a.Editor == "" {
			return a, func() tea.Msg { return reDiscoverMsg{} }
		}
		c := exec.Command(a.Editor, msg.path) //nolint:gosec
		return a, tea.ExecProcess(c, func(err error) tea.Msg { return editDoneMsg{err} })
```
```go
// in View, when a.newForm != nil, render the form as the body (overlay-style):
	if a.newForm != nil {
		body = a.newForm.View()
	}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./cli/tui/ -run TestNewModel -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cli/tui/newscenario.go cli/tui/app.go cli/tui/newscenario_test.go
git commit -m "feat(tui): scaffold a new scenario from a modal form"
```

---

### Task 7: Restyle scenarios list, detail, and history with the brand system

**Files:**
- Modify: `cli/tui/render.go` (use theme styles + badges; `RenderRun` header via `verdictBadge`)
- Modify: `cli/tui/detail.go` (panel border + badge in the header)
- Modify: `cli/tui/historyview.go` (bordered panel + run-strip + badges)
- Test: `cli/tui/historyview_test.go` (add run-strip case), `cli/tui/render_test.go` (keep passing)

**Interfaces:**
- Consumes: `badge`, `verdictBadge`, `glyph`, `panelStyle` (Task 1), `history.Record`, `history.FoldTrend`.
- Produces: `func runStrip(present []bool) string` — `●` for present, `○` for absent; used per finding.

- [ ] **Step 1: Write the failing test**

```go
// add to cli/tui/historyview_test.go
func TestRunStripMarksPresence(t *testing.T) {
	got := runStrip([]bool{true, true, false, true})
	want := "●●○●"
	if !strings.Contains(got, "○") || !strings.Contains(got, "●") {
		t.Fatalf("runStrip = %q, want marks like %q", got, want)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cli/tui/ -run TestRunStrip -v`
Expected: FAIL — undefined `runStrip`.

- [ ] **Step 3: Implement**

In `historyview.go`, add `runStrip`, wrap the trend table in `panelStyle`, and color status with `verdictBadge`/styles:

```go
func runStrip(present []bool) string {
	on := lipgloss.NewStyle().Foreground(ember)
	off := lipgloss.NewStyle().Foreground(fgDim)
	var b strings.Builder
	for _, p := range present {
		if p {
			b.WriteString(on.Render("●"))
		} else {
			b.WriteString(off.Render("○"))
		}
	}
	return b.String()
}
```

Then in `renderHistory`, render each trend row with the strip (derive `present` per finding by walking `records` in order and testing membership of `tr.ID`), wrap the whole body in `panelStyle(width, height, true).Render(...)`. In `render.go`, replace `titleStyle`/`verdictStyled`/`stepGlyph` usage with `verdictBadge`/`glyph` and append a trailing verdict glyph to each scenario list row where applicable. In `detail.go`, render the detail body inside `panelStyle(d.width, d.height, false)` and show `verdictBadge` in the header. Keep `render_test.go` and `detail_test.go` green (adjust expected substrings only if they assert exact styled bytes).

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./cli/tui/ -v`
Expected: PASS (all tui tests).

- [ ] **Step 5: Commit**

```bash
git add cli/tui/render.go cli/tui/detail.go cli/tui/historyview.go cli/tui/historyview_test.go cli/tui/render_test.go
git commit -m "feat(tui): restyle list, detail and history on the brand system"
```

---

### Task 8: Splash logo

**Files:**
- Create: `cli/tui/logo.go`
- Modify: `cli/tui/app.go` (show splash on launch, dismiss on any key)
- Test: `cli/tui/logo_test.go`, `cli/tui/app_test.go` (splash dismiss)

**Interfaces:**
- Produces: `func renderLogo(width int) string` — the ember fault dot over the deflecting line, `s h i n a r i`, tagline; `App` gains `showSplash bool` (true from `NewApp`), any key clears it.

- [ ] **Step 1: Write the failing test**

```go
// cli/tui/logo_test.go
package tui

import (
	"strings"
	"testing"
)

func TestLogoHasWordmarkAndTagline(t *testing.T) {
	out := renderLogo(80)
	if !strings.Contains(out, "shinari") && !strings.Contains(out, "s h i n a r i") {
		t.Fatalf("logo missing wordmark:\n%s", out)
	}
	if !strings.Contains(out, "resilience") {
		t.Fatalf("logo missing tagline:\n%s", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cli/tui/ -run TestLogo -v`
Expected: FAIL — undefined `renderLogo`.

- [ ] **Step 3: Implement `logo.go`**

```go
// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import "github.com/charmbracelet/lipgloss"

// renderLogo draws the favicon essence: the unbroken line deflecting around the
// ember fault dot, then the wordmark and tagline, centered to width.
func renderLogo(width int) string {
	line := lipgloss.NewStyle().Foreground(fg)
	dot := lipgloss.NewStyle().Foreground(ember).Bold(true)
	block := lipgloss.JoinVertical(lipgloss.Center,
		dot.Render("●"),
		line.Render("╶────────╮       ╭────────╴"),
		line.Render("         ╰───────╯"),
		"",
		lipgloss.NewStyle().Foreground(ember).Bold(true).Render("s h i n a r i"),
		lipgloss.NewStyle().Foreground(fgDim).Render("resilience integration testing"),
		"",
		lipgloss.NewStyle().Foreground(fgDim).Render("press any key to continue"),
	)
	return lipgloss.Place(width, lipgloss.Height(block)+2, lipgloss.Center, lipgloss.Center, block)
}
```

- [ ] **Step 4: Wire into `app.go`**

```go
// App struct: add
	showSplash bool
// in NewApp's returned App literal: showSplash: true,
// in Update's tea.KeyMsg branch, FIRST line:
	if a.showSplash {
		a.showSplash = false
		return a, nil
	}
// in View, FIRST line:
	if a.showSplash {
		return renderLogo(a.width)
	}
```

Update any existing `app_test.go` cases that send keys to first clear the splash (send one extra key, or set `a.showSplash = false` in the test setup). Add:

```go
// add to cli/tui/app_test.go (reuses appFromTempProject from Task 5)
func TestSplashDismissesOnKey(t *testing.T) {
	a := appFromTempProject(t)
	if !a.showSplash {
		t.Fatal("splash should be on at launch")
	}
	na, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if na.(App).showSplash {
		t.Fatal("any key should dismiss the splash")
	}
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./cli/tui/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cli/tui/logo.go cli/tui/app.go cli/tui/logo_test.go cli/tui/app_test.go
git commit -m "feat(tui): splash logo on launch"
```

---

### Task 9: Full verification and footer wiring

**Files:**
- Modify: `cli/tui/app.go` (footer key bar lists the new keys per mode; tally fed from `History`)
- Test: whole suite.

- [ ] **Step 1: Update the footer key sets in `App.View`**

In the scenarios branch, set `footerKeys = []key.Binding{a.keys.Tab, a.keys.Edit, a.keys.New, a.keys.Run, a.keys.Explain, a.keys.DryRun, a.keys.Quit}`. In the run branch, include `a.keys.Logs`. Render the status bar via `renderStatusBar(a.tally(), a.width)` where `tally()` counts pass/fail/findings from the latest `History` record (fall back to zeroes when empty).

```go
func (a App) tally() tallyCounts {
	if len(a.History) == 0 {
		return tallyCounts{}
	}
	last := a.History[len(a.History)-1]
	t := tallyCounts{Findings: len(last.Findings)}
	switch last.Verdict {
	case "PASSED":
		t.Pass = 1
	case "FAILED", "ERRORED":
		t.Fail = 1
	}
	return t
}
```

- [ ] **Step 2: Build, vet, and run the whole suite**

Run:
```bash
go build -o shinari ./cli
go vet ./...
go test ./...
```
Expected: build succeeds; vet clean; all tests PASS.

- [ ] **Step 3: Manual smoke (optional, documented for the reviewer)**

```bash
./shinari -p examples/quickstart tui
# splash → any key → Scenarios; ↵ opens $EDITOR; n scaffolds; r runs with l toggling logs; tab → History.
```

- [ ] **Step 4: Commit**

```bash
git add cli/tui/app.go
git commit -m "feat(tui): workbench footer keys and status tally"
```

---

## Notes for the implementer

- `RenderRun`'s signature is unchanged; Task 3 passes a scenario title as its header and renders the run verdict badge separately above it.
- Editing and scaffolding never touch core: they call `discover.Load` / `validate.Validate` (both in `core/`) directly from `cli/tui`, which is allowed (`cli → core`).
- `$EDITOR` is read by the CLI (`cmd_tui.go`) and injected as `App.Editor`; `cli/tui` itself never reads the environment.
- If a styled-output test is brittle, assert on the plain substring (label text, glyph rune), never on raw ANSI bytes — this matches the existing `render_test.go` style.
