// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWatchNonTTYPrintsStaticRender(t *testing.T) {
	// Produce a real journal by running a scenario with a finding.
	dir := writeFindingProject(t)
	out := filepath.Join(t.TempDir(), "reports")
	var so, se bytes.Buffer
	if code := run([]string{"--project", dir, "--out", out, "run"}, &so, &se, noEnv, noLookup); code != 0 {
		t.Fatalf("seed run exit=%d stderr=%s", code, se.String())
	}
	journal := filepath.Join(out, "journal.jsonl")
	if _, err := os.Stat(journal); err != nil {
		t.Fatalf("journal not written: %v", err)
	}

	// watch with a buffer (non-TTY) stdout falls back to a static render.
	so.Reset()
	se.Reset()
	if code := run([]string{"watch", journal}, &so, &se, noEnv, noLookup); code != 0 {
		t.Fatalf("watch exit=%d stderr=%s", code, se.String())
	}
	if !strings.Contains(so.String(), "known-gap") {
		t.Fatalf("watch should render the scenario, got:\n%s", so.String())
	}
}

func TestWatchMissingJournalErrors(t *testing.T) {
	var so, se bytes.Buffer
	if code := run([]string{"watch", filepath.Join(t.TempDir(), "nope.jsonl")}, &so, &se, noEnv, noLookup); code == 0 {
		t.Fatal("watch on a missing journal should exit non-zero")
	}
}
