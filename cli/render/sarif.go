// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"io"

	"github.com/shinari-dev/shinari/core/engine"
)

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string    `json:"id"`
	ShortDescription sarifText `json:"shortDescription"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             sarifText         `json:"message"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
}

// SARIF writes findings.sarif: a SARIF 2.1.0 log with one result per active
// finding, keyed by the finding's stable id so GitHub code scanning correlates
// it across runs. Now-passing findings are omitted (they are not active gaps).
func SARIF(w io.Writer, res engine.RunResult) error {
	rules := []sarifRule{}
	results := []sarifResult{}
	seen := map[string]bool{}
	for _, sc := range res.Scenarios {
		for _, f := range sc.Findings {
			if f.NowPasses {
				continue
			}
			if !seen[f.ID] {
				seen[f.ID] = true
				rules = append(rules, sarifRule{ID: f.ID, ShortDescription: sarifText{Text: f.Narrative}})
			}
			results = append(results, sarifResult{
				RuleID:              f.ID,
				Level:               "warning",
				Message:             sarifText{Text: f.Narrative},
				PartialFingerprints: map[string]string{"shinariFindingId/v1": f.ID},
			})
		}
	}
	doc := sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "shinari",
				InformationURI: "https://github.com/shinari-dev/shinari",
				Rules:          rules,
			}},
			Results: results,
		}},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
