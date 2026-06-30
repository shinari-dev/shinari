// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/shinari-dev/shinari/core/engine"
)

func TestSARIFEmitsFindingsAsResults(t *testing.T) {
	res := engine.RunResult{
		Scenarios: []engine.ScenarioResult{{
			Name: "checkout",
			Findings: []engine.FindingRecord{
				{ID: "sha-abc", Scenario: "checkout", Narrative: "cache outage drops requests"},
				{ID: "sha-xyz", Scenario: "checkout", Narrative: "fixed one", NowPasses: true},
			},
		}},
	}
	var b bytes.Buffer
	if err := SARIF(&b, res); err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name  string `json:"name"`
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID              string                `json:"ruleId"`
				Level               string                `json:"level"`
				Message             struct{ Text string } `json:"message"`
				PartialFingerprints map[string]string     `json:"partialFingerprints"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
		t.Fatalf("invalid SARIF JSON: %v\n%s", err, b.String())
	}
	if doc.Version != "2.1.0" {
		t.Fatalf("version: got %q want 2.1.0", doc.Version)
	}
	if len(doc.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(doc.Runs))
	}
	if len(doc.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 result (active finding only), got %d", len(doc.Runs[0].Results))
	}
	r := doc.Runs[0].Results[0]
	if r.RuleID != "sha-abc" || r.PartialFingerprints["shinariFindingId/v1"] != "sha-abc" {
		t.Fatalf("result identity wrong: %+v", r)
	}
	if r.Level != "warning" {
		t.Fatalf("level: got %q want warning", r.Level)
	}
}

func TestSARIFNoFindingsIsValidEmptyRun(t *testing.T) {
	var b bytes.Buffer
	if err := SARIF(&b, engine.RunResult{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b.Bytes(), []byte(`"results": []`)) {
		t.Fatalf("empty run should emit an empty results array, got:\n%s", b.String())
	}
}
