// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/shinari-dev/shinari/core/model"
)

func mkStep(t *testing.T, run, with, finding, id string) *model.Step {
	t.Helper()
	st := &model.Step{Run: run, Finding: finding, ID: id}
	if with != "" {
		if err := yaml.Unmarshal([]byte(with), &st.With); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func TestFindingIDExplicitWins(t *testing.T) {
	st := mkStep(t, "assert", "{ of: x, equals: 1 }", "gap", "my-id")
	if got := FindingID("sc", "verify", st); got != "my-id" {
		t.Fatalf("explicit id: got %q want my-id", got)
	}
}

func TestFindingIDStableAcrossNarrative(t *testing.T) {
	a := mkStep(t, "assert", "{ of: x, equals: 1 }", "first wording", "")
	b := mkStep(t, "assert", "{ of: x, equals: 1 }", "totally different", "")
	if FindingID("sc", "verify", a) != FindingID("sc", "verify", b) {
		t.Fatal("derived id changed when only the narrative changed")
	}
}

func TestFindingIDChangesWithCheck(t *testing.T) {
	a := mkStep(t, "assert", "{ of: x, equals: 1 }", "g", "")
	b := mkStep(t, "assert", "{ of: x, equals: 2 }", "g", "")
	if FindingID("sc", "verify", a) == FindingID("sc", "verify", b) {
		t.Fatal("derived id should change when the operand changes")
	}
}

func TestFindingIDIndependentOfKeyOrder(t *testing.T) {
	a := mkStep(t, "assert", "{ of: x, equals: 1 }", "g", "")
	b := mkStep(t, "assert", "{ equals: 1, of: x }", "g", "")
	if FindingID("sc", "verify", a) != FindingID("sc", "verify", b) {
		t.Fatal("derived id should not depend on with: key order")
	}
}

func TestReduceCarriesFindingID(t *testing.T) {
	t0 := time.Unix(0, 0)
	events := []Event{
		{Type: EvScenarioStarted, Scenario: "s", Time: t0},
		{Type: EvFindingRecorded, Scenario: "s", Step: "chk", Time: t0,
			Payload: map[string]any{"id": "sha-abc", "narrative": "gap", "detail": "boom"}},
		{Type: EvScenarioFinished, Scenario: "s", Time: t0,
			Payload: map[string]any{"verdict": "PASSED"}},
	}
	run := Reduce(events)
	if len(run.Scenarios) != 1 || len(run.Scenarios[0].Findings) != 1 {
		t.Fatalf("expected 1 scenario with 1 finding, got %+v", run.Scenarios)
	}
	if got := run.Scenarios[0].Findings[0].ID; got != "sha-abc" {
		t.Fatalf("finding id: got %q want sha-abc", got)
	}
}
