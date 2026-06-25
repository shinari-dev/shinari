// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shinari-dev/shinari/core/engine"
)

func TestModelFoldsLiveEvents(t *testing.T) {
	var m tea.Model = NewLive()
	m, _ = m.Update(EventMsg{Event: engine.Event{Type: engine.EvScenarioStarted, Scenario: "s", Time: time.Unix(0, 0)}})
	m, _ = m.Update(EventMsg{Event: engine.Event{Type: engine.EvFindingRecorded, Scenario: "s", Step: "chk", Time: time.Unix(0, 0),
		Payload: map[string]any{"id": "sha-x", "narrative": "gap"}}})
	if out := m.View(); !strings.Contains(out, "gap") {
		t.Fatalf("live view missing the finding:\n%s", out)
	}
}

func TestReplayScrub(t *testing.T) {
	events := []engine.Event{
		{Type: engine.EvScenarioStarted, Scenario: "s", Time: time.Unix(1, 0)},
		{Type: engine.EvFindingRecorded, Scenario: "s", Step: "c", Time: time.Unix(2, 0),
			Payload: map[string]any{"id": "sha-x", "narrative": "gap"}},
	}
	var m tea.Model = NewReplay(events)
	if !strings.Contains(m.View(), "gap") {
		t.Fatalf("replay at end should show the finding:\n%s", m.View())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if strings.Contains(m.View(), "gap") {
		t.Fatalf("scrubbed to start should hide the finding:\n%s", m.View())
	}
}

func TestLiveQuitsOnDone(t *testing.T) {
	var m tea.Model = NewLive()
	_, cmd := m.Update(DoneMsg{Res: engine.RunResult{}})
	if cmd == nil {
		t.Fatal("live mode should return a quit command on DoneMsg")
	}
}
