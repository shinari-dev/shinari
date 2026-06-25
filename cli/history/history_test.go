// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package history

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMissingIsEmpty(t *testing.T) {
	recs, err := Load(filepath.Join(t.TempDir(), "none.jsonl"))
	if err != nil {
		t.Fatalf("missing file must not error, got %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("missing file must load empty, got %d", len(recs))
	}
}

func TestAppendThenLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shinari-history.jsonl")
	r1 := Record{RunID: "a", Time: time.Unix(1, 0), Verdict: "PASSED",
		Findings: []Finding{{ID: "sha-1", Scenario: "s", Narrative: "gap"}}}
	r2 := Record{RunID: "b", Time: time.Unix(2, 0), Verdict: "FAILED"}
	if err := Append(path, r1); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, r2); err != nil {
		t.Fatal(err)
	}
	recs, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
	if recs[0].RunID != "a" || recs[1].RunID != "b" {
		t.Fatalf("records out of order: %+v", recs)
	}
	if len(recs[0].Findings) != 1 || recs[0].Findings[0].ID != "sha-1" {
		t.Fatalf("finding not round-tripped: %+v", recs[0])
	}
}

func TestFoldTrendOpen(t *testing.T) {
	recs := []Record{
		{Time: time.Unix(1, 0), Findings: []Finding{{ID: "sha-a", Scenario: "s", Narrative: "gap"}}},
		{Time: time.Unix(2, 0), Findings: []Finding{{ID: "sha-a", Scenario: "s", Narrative: "gap"}}},
	}
	trends := FoldTrend(recs)
	if len(trends) != 1 {
		t.Fatalf("expected 1 trend, got %d", len(trends))
	}
	tr := trends[0]
	if tr.ID != "sha-a" || tr.Runs != 2 || tr.Status != "open" {
		t.Fatalf("trend wrong: %+v", tr)
	}
}

func TestFoldTrendFixed(t *testing.T) {
	recs := []Record{
		{Time: time.Unix(1, 0), Findings: []Finding{{ID: "sha-a", Scenario: "s", Narrative: "gap"}}},
		{Time: time.Unix(2, 0), Findings: []Finding{{ID: "sha-a", Scenario: "s", Narrative: "gap", NowPasses: true}}},
	}
	if got := FoldTrend(recs)[0].Status; got != "fixed" {
		t.Fatalf("status: got %q want fixed", got)
	}
}

func TestFoldTrendGone(t *testing.T) {
	recs := []Record{
		{Time: time.Unix(1, 0), Findings: []Finding{{ID: "sha-a", Scenario: "s", Narrative: "gap"}}},
		{Time: time.Unix(2, 0)}, // no findings in the latest run
	}
	if got := FoldTrend(recs)[0].Status; got != "gone" {
		t.Fatalf("status: got %q want gone", got)
	}
}

func TestFoldTrendUnsortedInput(t *testing.T) {
	// records given newest-first must still fold chronologically.
	recs := []Record{
		{Time: time.Unix(2, 0), Findings: []Finding{{ID: "sha-a", Scenario: "s", NowPasses: true}}},
		{Time: time.Unix(1, 0), Findings: []Finding{{ID: "sha-a", Scenario: "s"}}},
	}
	if got := FoldTrend(recs)[0].Status; got != "fixed" {
		t.Fatalf("status with unsorted input: got %q want fixed", got)
	}
}
