// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const findingProjectYML = "apiVersion: shinari/v1\nkind: Project\nname: demo\nproviders:\n  sut: { source: clifake }\n"

const findingScenarioYML = `apiVersion: shinari/v1
kind: Scenario
name: known-gap
verify:
  - { run: sut.count, with: job, as: total }
  - run: assert
    finding: "totals are off by one"
    with: { of: "${.outputs.total.value}", equals: 999 }
`

const cleanScenarioYML = `apiVersion: shinari/v1
kind: Scenario
name: known-gap
verify:
  - { run: sut.count, with: job, as: total }
  - { run: assert, with: { of: "${.outputs.total.value}", equals: 1 } }
`

func writeFindingProject(t *testing.T) string {
	t.Helper()
	cliFail = false // clifake.count returns 1
	dir := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("project.yml", findingProjectYML)
	write("s.yml", findingScenarioYML)
	return dir
}

func TestGoldenUpdateThenMatch(t *testing.T) {
	dir := writeFindingProject(t)
	out := filepath.Join(t.TempDir(), "reports")
	var so, se bytes.Buffer

	// -u writes the ledger; a finding keeps the run green (exit 0).
	if code := run([]string{"--project", dir, "--out", out, "run", "-u"}, &so, &se, noEnv, noLookup); code != 0 {
		t.Fatalf("run -u exit=%d stderr=%s", code, se.String())
	}
	ledger := filepath.Join(dir, "shinari.findings.yml")
	data, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatalf("ledger not written: %v", err)
	}
	if !bytes.Contains(data, []byte("sha-")) {
		t.Fatalf("ledger should contain a derived id, got:\n%s", data)
	}

	// A second run with the ledger present matches: still exit 0, no drift.
	so.Reset()
	se.Reset()
	if code := run([]string{"--project", dir, "--out", out, "run"}, &so, &se, noEnv, noLookup); code != 0 {
		t.Fatalf("matching run exit=%d stdout=%s", code, so.String())
	}
}

func TestGoldenDriftWhenFindingRemoved(t *testing.T) {
	dir := writeFindingProject(t)
	out := filepath.Join(t.TempDir(), "reports")
	var so, se bytes.Buffer

	if code := run([]string{"--project", dir, "--out", out, "run", "-u"}, &so, &se, noEnv, noLookup); code != 0 {
		t.Fatalf("seed run -u exit=%d stderr=%s", code, se.String())
	}

	// Remove the finding: the ledger now expects a finding that no longer
	// fires -> drift -> non-zero exit.
	if err := os.WriteFile(filepath.Join(dir, "s.yml"), []byte(cleanScenarioYML), 0o644); err != nil {
		t.Fatal(err)
	}
	so.Reset()
	se.Reset()
	if code := run([]string{"--project", dir, "--out", out, "run"}, &so, &se, noEnv, noLookup); code == 0 {
		t.Fatalf("expected drift to fail the run, got exit 0; stdout=%s", so.String())
	}
}

func TestRunRecordAppendsHistory(t *testing.T) {
	dir := writeFindingProject(t)
	out := filepath.Join(t.TempDir(), "reports")
	var so, se bytes.Buffer

	for i := 0; i < 2; i++ {
		so.Reset()
		se.Reset()
		if code := run([]string{"--project", dir, "--out", out, "run", "--record"}, &so, &se, noEnv, noLookup); code != 0 {
			t.Fatalf("run --record exit=%d stderr=%s", code, se.String())
		}
	}

	data, err := os.ReadFile(filepath.Join(dir, "shinari-history.jsonl"))
	if err != nil {
		t.Fatalf("history not written: %v", err)
	}
	if n := bytes.Count(data, []byte("\n")); n != 2 {
		t.Fatalf("expected 2 history lines, got %d:\n%s", n, data)
	}
	if !bytes.Contains(data, []byte("sha-")) {
		t.Fatalf("history record should carry a finding id, got:\n%s", data)
	}
}

func TestRunWithoutRecordWritesNoHistory(t *testing.T) {
	dir := writeFindingProject(t)
	out := filepath.Join(t.TempDir(), "reports")
	var so, se bytes.Buffer
	if code := run([]string{"--project", dir, "--out", out, "run"}, &so, &se, noEnv, noLookup); code != 0 {
		t.Fatalf("run exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "shinari-history.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("history file must not exist without --record (err=%v)", err)
	}
}

