// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package golden

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shinari-dev/shinari/core/engine"
)

func TestReconcileMatch(t *testing.T) {
	g := File{Findings: []Entry{{ID: "sha-a", Scenario: "s", Narrative: "gap"}}}
	obs := []engine.FindingRecord{{ID: "sha-a", Scenario: "s", Narrative: "gap"}}
	if d := Reconcile(g, obs); !d.Empty() {
		t.Fatalf("expected no drift, got %+v", d)
	}
}

func TestReconcileUnexpected(t *testing.T) {
	g := File{}
	obs := []engine.FindingRecord{{ID: "sha-new", Scenario: "s", Narrative: "new gap"}}
	d := Reconcile(g, obs)
	if len(d.Unexpected) != 1 || d.Unexpected[0].ID != "sha-new" {
		t.Fatalf("expected 1 unexpected finding, got %+v", d)
	}
}

func TestReconcileMissing(t *testing.T) {
	g := File{Findings: []Entry{{ID: "sha-gone", Scenario: "s", Narrative: "old gap"}}}
	d := Reconcile(g, nil)
	if len(d.Missing) != 1 || d.Missing[0].ID != "sha-gone" {
		t.Fatalf("expected 1 missing finding, got %+v", d)
	}
}

func TestReconcileIgnoresNowPasses(t *testing.T) {
	g := File{}
	obs := []engine.FindingRecord{{ID: "sha-x", Scenario: "s", NowPasses: true}}
	if d := Reconcile(g, obs); !d.Empty() {
		t.Fatalf("a now-passing finding must not count as unexpected drift, got %+v", d)
	}
}

func TestFromObservedSortedAndSkipsNowPasses(t *testing.T) {
	obs := []engine.FindingRecord{
		{ID: "sha-b", Scenario: "s", Narrative: "two"},
		{ID: "sha-a", Scenario: "s", Narrative: "one"},
		{ID: "sha-z", Scenario: "s", NowPasses: true},
	}
	f := FromObserved(obs)
	if len(f.Findings) != 2 || f.Findings[0].ID != "sha-a" || f.Findings[1].ID != "sha-b" {
		t.Fatalf("expected 2 entries sorted by id, got %+v", f.Findings)
	}
}

func TestDriftReport(t *testing.T) {
	d := Drift{
		Unexpected: []engine.FindingRecord{{ID: "sha-new", Scenario: "s", Narrative: "new gap"}},
		Missing:    []Entry{{ID: "sha-gone", Scenario: "s", Narrative: "old gap"}},
	}
	var b strings.Builder
	if err := d.Report(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "sha-new") || !strings.Contains(out, "sha-gone") {
		t.Fatalf("report should mention both ids, got:\n%s", out)
	}
}

func TestLoadMissingIsNotAnError(t *testing.T) {
	_, exists, err := Load(filepath.Join(t.TempDir(), "nope.yml"))
	if err != nil {
		t.Fatalf("missing file must not error, got %v", err)
	}
	if exists {
		t.Fatal("missing file must report exists=false")
	}
}

func TestWriteThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shinari.findings.yml")
	want := File{Findings: []Entry{{ID: "sha-a", Scenario: "s", Narrative: "gap"}}}
	if err := Write(path, want); err != nil {
		t.Fatal(err)
	}
	got, exists, err := Load(path)
	if err != nil || !exists {
		t.Fatalf("load after write: exists=%v err=%v", exists, err)
	}
	if len(got.Findings) != 1 || got.Findings[0].ID != "sha-a" {
		t.Fatalf("round-trip mismatch: %+v", got.Findings)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "#") {
		t.Fatalf("expected a header comment, got:\n%s", data)
	}
}
