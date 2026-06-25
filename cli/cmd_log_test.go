// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogEmptyHistory(t *testing.T) {
	dir := writeFindingProject(t)
	var so, se bytes.Buffer
	if code := run([]string{"--project", dir, "log"}, &so, &se, noEnv, noLookup); code != 0 {
		t.Fatalf("log exit=%d stderr=%s", code, se.String())
	}
	if !strings.Contains(so.String(), "no history") {
		t.Fatalf("expected an empty-history hint, got:\n%s", so.String())
	}
}

func TestLogShowsFindingTrend(t *testing.T) {
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
	so.Reset()
	se.Reset()
	if code := run([]string{"--project", dir, "log"}, &so, &se, noEnv, noLookup); code != 0 {
		t.Fatalf("log exit=%d stderr=%s", code, se.String())
	}
	got := so.String()
	if !strings.Contains(got, "sha-") {
		t.Fatalf("log should show a finding id, got:\n%s", got)
	}
	if !strings.Contains(got, "open") {
		t.Fatalf("log should mark the active finding open, got:\n%s", got)
	}
}
