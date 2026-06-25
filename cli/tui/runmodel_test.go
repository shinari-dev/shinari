// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shinari-dev/shinari/core/engine"
)

func TestRunModelFoldsAndFinishes(t *testing.T) {
	r := newRun()
	r.setSize(80, 24)
	var afterCalled bool
	r.AfterRun = func(engine.RunResult) { afterCalled = true }

	r, _ = r.Update(EventMsg{Event: engine.Event{Type: engine.EvScenarioStarted, Scenario: "s", Time: time.Unix(0, 0)}})
	if strings.Contains(r.View(), "PASSED") {
		t.Fatal("should not show a verdict before DoneMsg")
	}
	r, _ = r.Update(DoneMsg{Res: engine.RunResult{Scenarios: []engine.ScenarioResult{{Name: "s", Verdict: engine.ScenarioPassed}}}})
	if !r.done {
		t.Fatal("DoneMsg should mark the run done")
	}
	if !afterCalled {
		t.Fatal("AfterRun should fire on completion (records history, writes reports)")
	}
	if !strings.Contains(r.View(), "s") {
		t.Fatalf("finished view should render the scenario:\n%s", r.View())
	}
}

func TestRunModelFullscreenAndFocus(t *testing.T) {
	r := newRun()
	r.setSize(80, 16)
	if !r.tp.bottomFocused() {
		t.Fatal("the log pane should be focused by default")
	}
	r, _ = r.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !r.tp.topFocused() {
		t.Fatal("tab should move focus to the scenarios pane")
	}
	r, _ = r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if !r.tp.full {
		t.Fatal("f should fullscreen the focused pane")
	}
	// Fullscreen the (focused top) pane hides the log pane entirely.
	if strings.Contains(r.View(), "logs") {
		t.Fatalf("fullscreen top should hide the log pane:\n%s", r.View())
	}
}

// The log pane (focused by default) tail-follows so the latest line stays in
// view, and scrolling up reaches the earliest line — the original bug.
func TestRunModelScrollsThroughAllLogs(t *testing.T) {
	r := newRun()
	r.setSize(80, 16)
	for i := 0; i < 60; i++ {
		r, _ = r.Update(EventMsg{Event: engine.Event{
			Type: engine.EvStepStarted,
			Step: fmt.Sprintf("step-%02d", i),
			Verb: "redis.kill",
			Time: time.Unix(0, 0),
		}})
	}
	v := r.View()
	if !strings.Contains(v, "step-59") {
		t.Fatalf("the log pane should tail-follow, keeping the latest line in view:\n%s", v)
	}
	if strings.Contains(v, "step-00") {
		t.Fatalf("the log must overflow its pane (first line off-screen), else nothing scrolls:\n%s", v)
	}
	for i := 0; i < 300; i++ { // scroll the focused (log) pane to the top
		r, _ = r.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	v = r.View()
	if !strings.Contains(v, "step-00") {
		t.Fatalf("scrolling up should reveal the earliest log line:\n%s", v)
	}
}

// run-all lists many scenarios in the top pane; that pane must scroll
// independently once it is focused (tab).
func TestRunModelScrollsThroughAllScenarios(t *testing.T) {
	r := newRun()
	r.setSize(80, 16)
	for i := 0; i < 30; i++ {
		r, _ = r.Update(EventMsg{Event: engine.Event{
			Type:     engine.EvScenarioStarted,
			Scenario: fmt.Sprintf("scenario-%02d", i),
			Time:     time.Unix(0, 0),
		}})
	}
	r, _ = r.Update(tea.KeyMsg{Type: tea.KeyTab}) // move focus to the scenarios pane
	v := r.View()
	if !strings.Contains(v, "scenario-00") {
		t.Fatalf("the scenarios pane should start at the top showing the first scenario:\n%s", v)
	}
	if strings.Contains(v, "scenario-29") {
		t.Fatalf("30 scenarios cannot all fit; the last must require scrolling:\n%s", v)
	}
	var reached bool
	for i := 0; i < 300 && !reached; i++ {
		r, _ = r.Update(tea.KeyMsg{Type: tea.KeyDown})
		reached = strings.Contains(r.View(), "scenario-29")
	}
	if !reached {
		t.Fatalf("scrolling the scenarios pane down should reach the last scenario:\n%s", r.View())
	}
}
