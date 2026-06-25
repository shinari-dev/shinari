// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package history is the durable run-record store at the CLI edge: an
// append-only NDJSON log of compact per-run records, beside the ephemeral
// shinari-out/ scratch dir. History is observational and never feeds a verdict;
// core never sees it.
package history

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"time"
)

// Record is one run's compact, durable history entry.
type Record struct {
	RunID    string    `json:"runId"`
	Time     time.Time `json:"time"`
	Verdict  string    `json:"verdict"`
	Findings []Finding `json:"findings,omitempty"`
}

// Finding is one finding as captured in a run-record, keyed by its stable id.
type Finding struct {
	ID        string `json:"id"`
	Scenario  string `json:"scenario"`
	Narrative string `json:"narrative"`
	NowPasses bool   `json:"nowPasses,omitempty"`
}

// Append writes one record as a JSON line to path, creating it if absent.
func Append(path string, rec Record) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// Load reads all run-records from an NDJSON history file. A missing file is not
// an error: it returns no records.
func Load(path string) ([]Record, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// Trend is one finding's history across recorded runs.
type Trend struct {
	ID        string
	Scenario  string
	Narrative string
	Runs      int       // records where the finding appeared active
	FirstTime time.Time // first appearance (active or not)
	LastTime  time.Time // last active appearance
	Status    string    // "open", "fixed", or "gone"
}

// FoldTrend reduces run-records into one Trend per finding id, in first-seen
// order. Input may be in any order; it is folded chronologically.
func FoldTrend(records []Record) []Trend {
	sorted := append([]Record(nil), records...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Time.Before(sorted[j].Time) })

	agg := map[string]*Trend{}
	order := []string{}
	for _, rec := range sorted {
		for _, f := range rec.Findings {
			t, ok := agg[f.ID]
			if !ok {
				t = &Trend{ID: f.ID, Scenario: f.Scenario, FirstTime: rec.Time}
				agg[f.ID] = t
				order = append(order, f.ID)
			}
			if f.Narrative != "" {
				t.Narrative = f.Narrative
			}
			if !f.NowPasses {
				t.Runs++
				t.LastTime = rec.Time
			}
		}
	}

	if len(sorted) > 0 {
		latest := sorted[len(sorted)-1]
		active := map[string]bool{}
		passed := map[string]bool{}
		for _, f := range latest.Findings {
			if f.NowPasses {
				passed[f.ID] = true
			} else {
				active[f.ID] = true
			}
		}
		for id, t := range agg {
			switch {
			case active[id]:
				t.Status = "open"
			case passed[id]:
				t.Status = "fixed"
			default:
				t.Status = "gone"
			}
		}
	}

	out := make([]Trend, 0, len(order))
	for _, id := range order {
		out = append(out, *agg[id])
	}
	return out
}
