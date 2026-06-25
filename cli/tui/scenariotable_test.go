// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestRelAge(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Second: "now",
		5 * time.Minute:  "5m",
		3 * time.Hour:    "3h",
		50 * time.Hour:   "2d",
	}
	for d, want := range cases {
		if got := relAge(d); got != want {
			t.Fatalf("relAge(%s) = %q, want %q", d, got, want)
		}
	}
}

func TestRenderScenarioRowColumns(t *testing.T) {
	row := stripANSI(renderScenarioRow("checkout", []string{"slow", "redis"}, "PASSED", "2m", false, 80))
	for _, want := range []string{"checkout", "slow", "PASSED", "2m"} {
		if !strings.Contains(row, want) {
			t.Fatalf("row should show %q: %q", want, row)
		}
	}
	if renderScenarioRow("checkout", nil, "", "", true, 80) == renderScenarioRow("checkout", nil, "", "", false, 80) {
		t.Fatal("selected row should render differently from unselected")
	}
}

// A row with long, multiple tags must stay one line no wider than the pane —
// otherwise a Height()==1 delegate renders a staggered, overlapping mess.
func TestRenderScenarioRowSingleLineWithinWidth(t *testing.T) {
	for _, w := range []int{80, 120, 240} {
		inner := w - 2
		row := renderScenarioRow(
			"some-very-long-scenario-name-that-exceeds",
			[]string{"data-loss-prevention", "recovery", "network-fault-tolerance"},
			"ERRORED", "12h", true, inner)
		if strings.Contains(row, "\n") {
			t.Fatalf("w=%d: row must be a single line, got:\n%q", w, row)
		}
		if got := lipgloss.Width(row); got > inner {
			t.Fatalf("w=%d: row width %d exceeds inner %d", w, got, inner)
		}
	}
}

func TestScenarioHeaderHasColumns(t *testing.T) {
	h := stripANSI(scenarioHeader(80))
	for _, c := range []string{"NAME", "TAGS", "STATUS", "LAST"} {
		if !strings.Contains(h, c) {
			t.Fatalf("header missing %q: %q", c, h)
		}
	}
}
