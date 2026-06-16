// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import "time"

// CheckVerdict is the per-check outcome.
type CheckVerdict string

const (
	CheckPass    CheckVerdict = "PASS"
	CheckFail    CheckVerdict = "FAIL"
	CheckSkip    CheckVerdict = "SKIP"
	CheckFinding CheckVerdict = "FINDING"
)

// ScenarioVerdict maps onto familiar test-runner verdicts.
type ScenarioVerdict string

const (
	ScenarioPassed       ScenarioVerdict = "PASSED"
	ScenarioFailed       ScenarioVerdict = "FAILED"
	ScenarioErrored      ScenarioVerdict = "ERRORED"
	ScenarioInconclusive ScenarioVerdict = "INCONCLUSIVE"
)

// ExitCode is the front-end verdict→exit-code mapping — exposed as data so each
// front end applies its own policy (the CLI uses it as a process code).
func (v ScenarioVerdict) ExitCode() int {
	switch v {
	case ScenarioPassed:
		return 0
	case ScenarioFailed:
		return 1
	case ScenarioErrored:
		return 2
	case ScenarioInconclusive:
		return 3
	}
	return 1
}

// StepResult is one executed (or skipped) step.
type StepResult struct {
	Section    string         `json:"section"`
	Phase      string         `json:"phase,omitempty"`
	Run        string         `json:"run"`
	Desc       string         `json:"desc,omitempty"`
	Verdict    CheckVerdict   `json:"verdict"`
	Finding    string         `json:"finding,omitempty"`
	Err        string         `json:"error,omitempty"`
	TimedOut   bool           `json:"timedOut,omitempty"`
	SkipReason string         `json:"skipReason,omitempty"`
	Captured   map[string]any `json:"captured,omitempty"`
	Start      time.Time      `json:"start"`
	End        time.Time      `json:"end"`
}

// Label is the human label for a step: its desc, falling back to the verb.
func (s StepResult) Label() string {
	if s.Desc != "" {
		return s.Desc
	}
	return s.Run
}

// FindingRecord is one ledger entry: a known, expected failure.
type FindingRecord struct {
	Scenario  string `json:"scenario"`
	Narrative string `json:"narrative"` // the finding: text
	Check     string `json:"check"`     // desc or run of the check
	Detail    string `json:"detail"`    // the observed failure it expects
	NowPasses bool   `json:"nowPasses"` // gap fixed → promote to hard assertion
}

// ScenarioResult is the terminal outcome of one scenario, rich enough for
// every front end to render without re-running anything.
type ScenarioResult struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Suite       string          `json:"suite,omitempty"`
	Verdict     ScenarioVerdict `json:"verdict"`
	Reason      string          `json:"reason,omitempty"` // why FAILED/ERRORED/INCONCLUSIVE
	Steps       []StepResult    `json:"steps"`
	Findings    []FindingRecord `json:"findings,omitempty"`
	Injected    []string        `json:"injected,omitempty"` // faults injected during method
	Held        []string        `json:"held,omitempty"`     // assertions that held
	Start       time.Time       `json:"start"`
	End         time.Time       `json:"end"`
}

// RunResult is the whole run.
type RunResult struct {
	Scenarios []ScenarioResult `json:"scenarios"`
	Start     time.Time        `json:"start"`
	End       time.Time        `json:"end"`
}

// Verdict rolls scenario verdicts up to the run level: the worst wins,
// in the order ERRORED > FAILED > INCONCLUSIVE > PASSED.
func (r RunResult) Verdict() ScenarioVerdict {
	rank := map[ScenarioVerdict]int{
		ScenarioPassed: 0, ScenarioInconclusive: 1, ScenarioFailed: 2, ScenarioErrored: 3,
	}
	worst := ScenarioPassed
	for _, sc := range r.Scenarios {
		if rank[sc.Verdict] > rank[worst] {
			worst = sc.Verdict
		}
	}
	return worst
}
