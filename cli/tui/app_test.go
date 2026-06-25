// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/engine"
	"github.com/shinari-dev/shinari/core/model"
)

func testSet() *discover.Set {
	return &discover.Set{
		Root:    "/tmp/p",
		Project: &model.Project{},
		Scenarios: []*model.Scenario{
			{Header: model.Header{Name: "checkout-resilience"}},
			{Header: model.Header{Name: "cache-outage"}},
		},
	}
}

// appFromTempProject scaffolds one scenario in a temp project and builds an App.
func appFromTempProject(t *testing.T) App {
	t.Helper()
	root := t.TempDir()
	writeProject(t, root)
	if _, err := writeScenario(root, "s", "demo", "minimal"); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	set, err := discover.Load(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	return NewApp(set)
}

func TestDeleteScenarioConfirmFlow(t *testing.T) {
	a := appFromTempProject(t)
	a.showSplash = false
	var m tea.Model = a
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	sc := m.(App).Selected()
	if sc == nil || sc.File == "" {
		t.Fatalf("need a selected scenario with a file, got %v", sc)
	}
	// ⇧D arms the confirmation; the file is untouched yet.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	if m.(App).confirmDelete == nil {
		t.Fatal("⇧D should arm the delete confirmation")
	}
	if _, err := os.Stat(sc.File); err != nil {
		t.Fatalf("file must still exist before confirming: %v", err)
	}
	// n cancels, file survives.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.(App).confirmDelete != nil {
		t.Fatal("n should cancel the delete")
	}
	if _, err := os.Stat(sc.File); err != nil {
		t.Fatalf("cancel must not delete the file: %v", err)
	}
	// ⇧D then y deletes the file from disk and clears the confirmation.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.(App).confirmDelete != nil {
		t.Fatal("y should clear the pending delete")
	}
	if _, err := os.Stat(sc.File); !os.IsNotExist(err) {
		t.Fatalf("y should delete the scenario file, stat err = %v", err)
	}
}

func TestReDiscoverUpdatesValidateSummary(t *testing.T) {
	a := appFromTempProject(t)
	na, _ := a.Update(reDiscoverMsg{})
	if na.(App).validateSummary == "" {
		t.Fatal("re-discover should populate a validate summary")
	}
}

func TestReDiscoverRebindsDetail(t *testing.T) {
	a := appFromTempProject(t)
	a.showSplash = false
	var m tea.Model = a
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(reDiscoverMsg{})
	app := m.(App)
	// Detail must track the reloaded set and selection, and render content.
	if app.detail.set != app.set {
		t.Fatal("re-discover should re-bind the detail to the reloaded set")
	}
	if app.detail.scenario != app.Selected() {
		t.Fatal("re-discover should re-bind the detail to the current selection")
	}
	if strings.TrimSpace(stripANSI(app.detail.inner(app.Selected()))) == "" {
		t.Fatal("detail should not be blank after re-discover")
	}
}

func TestAppStartsOnScenariosWithSelection(t *testing.T) {
	a := NewApp(testSet())
	var m tea.Model = a
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app := m.(App)
	if app.Tab() != "scenarios" {
		t.Fatalf("tab: got %q want scenarios", app.Tab())
	}
	if app.Selected() == nil || app.Selected().Name != "checkout-resilience" {
		t.Fatalf("expected first scenario selected, got %v", app.Selected())
	}
}

func scenariosApp(t *testing.T) App {
	t.Helper()
	a := NewApp(testSet())
	a.showSplash = false
	var m tea.Model = a
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m.(App)
}

func TestScenariosTabTogglesFocus(t *testing.T) {
	a := scenariosApp(t)
	if !a.scen.topFocused() {
		t.Fatal("table should be focused by default")
	}
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !m.(App).scen.bottomFocused() {
		t.Fatal("tab should move focus to the detail pane")
	}
}

func TestScenariosFullscreenTogglesAndHidesOther(t *testing.T) {
	a := scenariosApp(t) // table focused
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	app := m.(App)
	if !app.scen.full {
		t.Fatal("f should fullscreen the focused pane")
	}
	if strings.Contains(stripANSI(app.View()), "Overview") {
		t.Fatalf("fullscreen table should hide the detail pane:\n%s", stripANSI(app.View()))
	}
}

func TestTableFocusedDownMovesSelection(t *testing.T) {
	a := scenariosApp(t) // table focused
	first := a.Selected().Name
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.(App).Selected().Name == first {
		t.Fatal("down with the table focused should move the selection")
	}
}

func TestDetailFocusedDownDoesNotMoveSelection(t *testing.T) {
	a := scenariosApp(t)
	a.scen.toggleFocus() // detail focused
	first := a.Selected().Name
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.(App).Selected().Name != first {
		t.Fatal("down with the detail focused should scroll, not change selection")
	}
}

func TestScenariosScreenIsStackedTable(t *testing.T) {
	a := NewApp(testSet())
	a.showSplash = false
	var m tea.Model = a
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := stripANSI(m.(App).View())
	for _, want := range []string{"NAME", "TAGS", "STATUS", "Scenarios", "Overview"} {
		if !strings.Contains(view, want) {
			t.Fatalf("scenarios view missing %q:\n%s", want, view)
		}
	}
}

func TestShiftTabCyclesProjectAndScenarios(t *testing.T) {
	a := NewApp(testSet())
	a.showSplash = false
	var m tea.Model = a // starts on scenarios
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := m.(App).Tab(); got != "project" {
		t.Fatalf("1st ⇧⇥: want project, got %q", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := m.(App).Tab(); got != "scenarios" {
		t.Fatalf("2nd ⇧⇥: want scenarios, got %q", got)
	}
}

func TestSearchFiltersScenarios(t *testing.T) {
	a := NewApp(testSet()) // checkout-resilience, cache-outage
	a.showSplash = false
	var m tea.Model = a
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if m.(App).list.FilterState() != list.Filtering {
		t.Fatal("/ should open the search filter")
	}
	var cmd tea.Cmd
	for _, r := range "cache" {
		m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// The list filters asynchronously: the runtime expands the returned batch and
	// delivers each result (incl. FilterMatchesMsg). Simulate that here.
	if cmd != nil {
		if batch, ok := cmd().(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil {
					m, _ = m.Update(c())
				}
			}
		}
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if n := len(m.(App).list.VisibleItems()); n != 1 {
		t.Fatalf("searching 'cache' should leave 1 match, got %d", n)
	}
	if sel := m.(App).Selected(); sel == nil || !strings.Contains(sel.Name, "cache") {
		t.Fatalf("after searching 'cache', expected cache-outage, got %v", sel)
	}
}

func TestRunAllRunsVisibleScenarios(t *testing.T) {
	a := NewApp(testSet()) // checkout-resilience, cache-outage
	a.showSplash = false
	var got []string
	a.RunSetFn = func(_ context.Context, targets []string, _ func(engine.Event)) (engine.RunResult, error) {
		got = targets
		return engine.RunResult{}, nil
	}
	var m tea.Model = a
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if nm.(App).run == nil || cmd == nil {
		t.Fatal("shift+r should start a run over the visible scenarios")
	}
	cmd() // execute the run command to capture the targets
	if len(got) != 2 {
		t.Fatalf("run-all should target both visible scenarios, got %v", got)
	}
}

func TestProjectContentShowsNameAndProviders(t *testing.T) {
	p := &model.Project{
		Header:    model.Header{Name: "proj", Description: "desc"},
		Providers: map[string]model.ProviderConfig{"exec": {}, "http": {}},
	}
	out := projectContent(p)
	for _, w := range []string{"proj", "desc", "Providers", "exec", "http"} {
		if !strings.Contains(out, w) {
			t.Fatalf("project content missing %q:\n%s", w, out)
		}
	}
}

func TestAppQuits(t *testing.T) {
	a := NewApp(testSet())
	a.showSplash = false
	var m tea.Model = a
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q should quit")
	}
}

func TestViewFitsTerminalHeight(t *testing.T) {
	sizes := []struct{ w, h int }{{100, 30}, {80, 24}, {120, 40}}
	keys := map[string]rune{"overview": 'o', "explain": 'e', "dryrun": 'd'}
	for _, s := range sizes {
		a := NewApp(testSet())
		a.showSplash = false
		a.detail.ExplainFn = func(*model.Scenario) string { return strings.Repeat("explain line\n", 200) }
		var m tea.Model = a
		m, _ = m.Update(tea.WindowSizeMsg{Width: s.w, Height: s.h})
		for name, k := range keys {
			m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{k}})
			if got := lipgloss.Height(m2.View()); got > s.h {
				t.Fatalf("%s at %dx%d: rendered %d rows > terminal %d", name, s.w, s.h, got, s.h)
			}
		}
		// project tab too
		mh, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		if got := lipgloss.Height(mh.View()); got > s.h {
			t.Fatalf("project at %dx%d: rendered %d rows > terminal %d", s.w, s.h, got, s.h)
		}
	}
}

func TestDryRunStreamsThroughRunView(t *testing.T) {
	a := appFromTempProject(t)
	a.showSplash = false
	a.DryStreamFn = func(_ context.Context, sc *model.Scenario, _ func(engine.Event)) (engine.RunResult, error) {
		return engine.RunResult{Scenarios: []engine.ScenarioResult{{Name: sc.Name, Verdict: engine.ScenarioPassed}}}, nil
	}
	var m tea.Model = a
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.(App).run == nil {
		t.Fatal("pressing d should start a streaming run sub-model")
	}
	if !m.(App).run.dry {
		t.Fatal("the run should be flagged as a dry-run")
	}
	if cmd == nil {
		t.Fatal("dry-run should return a command that drives the engine")
	}
}

func TestShiftEStartsEdit(t *testing.T) {
	a := appFromTempProject(t)
	a.showSplash = false
	a.Editor = "vi"
	var m tea.Model = a
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}}); cmd == nil {
		t.Fatal("shift+e (E) should launch the editor on the selected scenario")
	}
}

func TestShiftPEditsProject(t *testing.T) {
	a := appFromTempProject(t)
	a.showSplash = false
	a.Editor = "vi"
	var m tea.Model = a
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}}); cmd == nil {
		t.Fatal("shift+p (P) should open the project file in the editor")
	}
}

func TestEditSelectedFallsBackToVi(t *testing.T) {
	a := appFromTempProject(t) // Editor == "" (no $EDITOR)
	_, cmd := a.editSelected()
	if cmd == nil {
		t.Fatal("editSelected should fall back to a default editor when $EDITOR is unset")
	}
}

func TestSplashDismissesOnKey(t *testing.T) {
	a := NewApp(testSet())
	if !a.showSplash {
		t.Fatal("splash should be on at launch")
	}
	na, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if na.(App).showSplash {
		t.Fatal("any key should dismiss the splash")
	}
}
