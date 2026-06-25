// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"
	"testing"

	"github.com/shinari-dev/shinari/core/engine"
)

func TestRenderRunShowsScenarioStepsAndFindings(t *testing.T) {
	run := engine.RunResult{
		Scenarios: []engine.ScenarioResult{{
			Name:    "checkout-resilience",
			Verdict: engine.ScenarioPassed,
			Steps: []engine.StepResult{
				{Section: "verify", Run: "assert", Desc: "exactly once", Verdict: engine.CheckPass},
			},
			Findings: []engine.FindingRecord{
				{ID: "sha-abc123", Scenario: "checkout-resilience", Narrative: "cache outage drops requests"},
			},
		}},
	}
	out := RenderRun(run, "shinari — replay", 0)
	for _, want := range []string{"checkout-resilience", "exactly once", "cache outage drops requests", "sha-abc123"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderRunEmpty(t *testing.T) {
	out := RenderRun(engine.RunResult{}, "shinari — running", 0)
	if !strings.Contains(out, "waiting") {
		t.Fatalf("empty render should hint waiting, got:\n%s", out)
	}
}
