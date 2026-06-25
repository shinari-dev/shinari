// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shinari-dev/shinari/cli/history"
	"github.com/shinari-dev/shinari/core/model"
)

func TestDetailOverviewScrolls(t *testing.T) {
	d := newDetail(testSet())
	d.setSize(40, 8) // small viewport
	d.scenario = &model.Scenario{Header: model.Header{Name: "s", Description: strings.Repeat("long ", 200)}}
	d = d.gotoSub(subOverview) // load overview into the viewport
	before := d.vp.YOffset
	d2, _ := d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d2.vp.YOffset == before {
		t.Fatalf("Overview should scroll through the viewport (offset %d unchanged)", before)
	}
}

func TestScenarioShape(t *testing.T) {
	sc := &model.Scenario{
		Setup:       []model.Step{{Run: "docker.up"}},
		SteadyState: []model.Step{{Run: "http.get"}},
		Method: []model.Phase{{Phase: "p", Steps: []model.Step{
			{Run: "exec.run", Effect: "outage"},
			{Run: "assert", Finding: "F1"},
		}}},
	}
	got := scenarioShape(sc)
	for _, want := range []string{"4 steps", "1 faults", "1 findings"} {
		if !strings.Contains(got, want) {
			t.Fatalf("shape %q missing %q", got, want)
		}
	}
}

func TestLastRunLine(t *testing.T) {
	now := time.Unix(1000, 0)
	recs := []history.Record{
		{Time: now.Add(-5 * time.Minute), Verdict: "FAILED", Duration: 4200 * time.Millisecond, Scenarios: []string{"s"}},
	}
	line := stripANSI(lastRunLine(recs, "s", now))
	for _, want := range []string{"FAILED", "5m ago", "4.2s"} {
		if !strings.Contains(line, want) {
			t.Fatalf("last-run line %q missing %q", line, want)
		}
	}
	if got := stripANSI(lastRunLine(nil, "s", now)); got != "—" {
		t.Fatalf("never-run want —, got %q", got)
	}
}

func TestDetailHistorySelectable(t *testing.T) {
	name := testSet().Scenarios[0].Name
	d := newDetail(testSet())
	d.setSize(60, 16)
	d.scenario = testSet().Scenarios[0]
	d.sub = subRuns
	d.runs = []history.Record{
		{Time: time.Unix(2000, 0), Verdict: "FAILED", Duration: 3 * time.Second,
			Findings: []history.Finding{{ID: "F1", Scenario: name, Narrative: "cache miss"}}},
		{Time: time.Unix(1000, 0), Verdict: "PASSED"},
	}
	// The first (selected) run shows its findings.
	c := stripANSI(d.runsContent())
	for _, want := range []string{"FAILED", "F1", "cache miss"} {
		if !strings.Contains(c, want) {
			t.Fatalf("selected run detail should show %q:\n%s", want, c)
		}
	}
	// ↓ selects the second run; its detail shows PASSED and no findings.
	d2, _ := d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d2.runCursor != 1 {
		t.Fatalf("down should move the run cursor to 1, got %d", d2.runCursor)
	}
	c2 := stripANSI(d2.runsContent())
	if !strings.Contains(c2, "PASSED") || !strings.Contains(c2, "no findings") {
		t.Fatalf("second run detail should show PASSED + no findings:\n%s", c2)
	}
}

func TestDetailExplainView(t *testing.T) {
	d := newDetail(testSet())
	d.setSize(60, 20)
	d.ExplainFn = func(sc *model.Scenario) string { return "TIMELINE for " + sc.Name }
	sc := testSet().Scenarios[0]
	d.scenario = sc

	// ←/→ cycle the sub-tabs: one step right from Overview reaches Explain.
	d2, _ := d.Update(tea.KeyMsg{Type: tea.KeyRight})
	if !strings.Contains(d2.inner(sc), "TIMELINE for "+sc.Name) {
		t.Fatalf("explain view missing timeline:\n%s", d2.inner(sc))
	}
}

func TestDetailSubTabsCycleBothWays(t *testing.T) {
	d := newDetail(testSet())
	d.setSize(60, 20)
	d.scenario = testSet().Scenarios[0]
	// left from Overview wraps to History (the last sub-tab).
	if got, _ := d.Update(tea.KeyMsg{Type: tea.KeyLeft}); got.sub != subRuns {
		t.Fatalf("left from Overview should wrap to History, got %d", got.sub)
	}
	// right from Overview goes to Explain.
	if got, _ := d.Update(tea.KeyMsg{Type: tea.KeyRight}); got.sub != subExplain {
		t.Fatalf("right from Overview should go to Explain, got %d", got.sub)
	}
}
