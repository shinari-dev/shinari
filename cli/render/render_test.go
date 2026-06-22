// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shinari-dev/shinari/core/engine"
)

func sample() engine.RunResult {
	t0 := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	return engine.RunResult{
		Start: t0, End: t0.Add(90 * time.Second),
		Scenarios: []engine.ScenarioResult{{
			Name: "worker-killed", Suite: "data-loss", Verdict: engine.ScenarioPassed,
			Description: "job survives SIGKILL",
			Start:       t0, End: t0.Add(80 * time.Second),
			Injected: []string{"docker.kill worker-a"},
			Held:     []string{"exactly once"},
			Steps: []engine.StepResult{
				{Section: "setup", Run: "docker.up", Verdict: engine.CheckPass, Start: t0, End: t0.Add(time.Second)},
				{Section: "verify", Run: "assert", Desc: "exactly once", Verdict: engine.CheckPass, Start: t0, End: t0.Add(2 * time.Second)},
				{Section: "verify", Run: "assert", Desc: "no leak", Verdict: engine.CheckFinding,
					Finding: "connections leak after kill", Start: t0, End: t0.Add(3 * time.Second)},
			},
			Findings: []engine.FindingRecord{{
				Scenario: "worker-killed", Narrative: "connections leak after kill",
				Check: "no leak", Detail: "expected 0 == 12",
			}},
		}},
	}
}

func TestTSV(t *testing.T) {
	var buf bytes.Buffer
	if err := TSV(&buf, sample()); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %d:\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[2], "exactly once\tPASS") {
		t.Errorf("row: %q", lines[2])
	}
	if !strings.Contains(lines[3], "FINDING") {
		t.Errorf("row: %q", lines[3])
	}
}

func TestResultsJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := ResultsJSON(&buf, sample()); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["verdict"] != "PASSED" || doc["exitCode"] != float64(0) {
		t.Errorf("doc: %v", doc)
	}
}

func TestJUnitShape(t *testing.T) {
	var buf bytes.Buffer
	if err := JUnit(&buf, sample()); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	for _, want := range []string{
		`<testsuite name="worker-killed"`,
		`tests="3"`,
		`failures="0"`,
		`FINDING (expected failure, ledger-tracked): connections leak after kill`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("junit.xml missing %q:\n%s", want, s)
		}
	}
}

func TestFindingsReport(t *testing.T) {
	var buf bytes.Buffer
	if err := FindingsReport(&buf, sample()); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	for _, want := range []string{"**Injected**", "**Held**", "**Gapped**", "connections leak after kill", "expected 0 == 12"} {
		if !strings.Contains(s, want) {
			t.Errorf("findings.md missing %q:\n%s", want, s)
		}
	}
}

func TestJournalIsTheEventStream(t *testing.T) {
	events := []engine.Event{
		{Type: engine.EvScenarioStarted, Scenario: "s"},
		{Type: engine.EvScenarioFinished, Scenario: "s", Payload: map[string]any{"verdict": "PASSED"}},
	}
	var buf bytes.Buffer
	if err := Journal(&buf, events); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("journal lines = %d", len(lines))
	}
	var e engine.Event
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil || e.Type != engine.EvScenarioStarted {
		t.Errorf("line 0: %v %v", e, err)
	}
}

func TestConsoleStreams(t *testing.T) {
	var buf bytes.Buffer
	c := &Console{W: &buf}
	c.Emit(engine.Event{Type: engine.EvScenarioStarted, Scenario: "s"})
	c.Emit(engine.Event{Type: engine.EvFindingRecorded, Step: "no leak",
		Payload: map[string]any{"narrative": "connections leak after kill"}})
	c.Emit(engine.Event{Type: engine.EvStepFailed, Step: "no leak",
		Payload: map[string]any{"verdict": "FINDING"}})
	c.Emit(engine.Event{Type: engine.EvStepFailed, Step: "boom",
		Payload: map[string]any{"verdict": "FAIL", "error": "kaput"}})
	c.Emit(engine.Event{Type: engine.EvScenarioFinished,
		Payload: map[string]any{"verdict": "FAILED", "reason": "boom failed"}})
	s := buf.String()
	if !strings.Contains(s, "◆ no leak") || !strings.Contains(s, "FINDING: connections leak after kill") {
		t.Errorf("finding line missing:\n%s", s)
	}
	if !strings.Contains(s, "✗ boom — kaput") {
		t.Errorf("fail line missing:\n%s", s)
	}
	if !strings.Contains(s, "✘ FAILED") || !strings.Contains(s, "1 finding held") {
		t.Errorf("verdict line missing:\n%s", s)
	}
}
