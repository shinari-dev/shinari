// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestExplainShowsTimeline(t *testing.T) {
	dir := project(t, false)
	var out, errOut bytes.Buffer
	if code := run([]string{"-p", dir, "explain", "exactly-once"}, &out, &errOut, noEnv, noLookup); code != 0 {
		t.Fatalf("code = %d: %s", code, errOut.String())
	}
	s := out.String()
	// The scenario name, the sections it has, and a resolved verb with its kind.
	for _, want := range []string{"exactly-once", "setup:", "verify:", "sut.up", "[action]", "sut.count", "[probe]", "[assertion]"} {
		if !strings.Contains(s, want) {
			t.Errorf("explain output missing %q:\n%s", want, s)
		}
	}
}

func TestExplainUnknownTargetIsUsageError(t *testing.T) {
	dir := project(t, false)
	var out, errOut bytes.Buffer
	if code := run([]string{"-p", dir, "explain", "nope"}, &out, &errOut, noEnv, noLookup); code != exitUsage {
		t.Fatalf("code = %d, want %d", code, exitUsage)
	}
}

func TestRunKeepUpFlagAccepted(t *testing.T) {
	dir := project(t, false)
	var out, errOut bytes.Buffer
	if code := run([]string{"-p", dir, "-o", t.TempDir(), "run", "--keep-up"}, &out, &errOut, noEnv, noLookup); code != 0 {
		t.Fatalf("code = %d: %s%s", code, out.String(), errOut.String())
	}
}

func TestRunVerboseShowsValuesAndDurations(t *testing.T) {
	dir := project(t, false)
	var out, errOut bytes.Buffer
	if code := run([]string{"-p", dir, "-o", t.TempDir(), "run", "-v"}, &out, &errOut, noEnv, noLookup); code != 0 {
		t.Fatalf("code = %d: %s%s", code, out.String(), errOut.String())
	}
	s := out.String()
	if !strings.Contains(s, "ms)") {
		t.Errorf("verbose run did not show step durations:\n%s", s)
	}
}
