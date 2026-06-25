// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestResolveEditor(t *testing.T) {
	if got := resolveEditor(func(string) string { return "" }); got != "vi" {
		t.Fatalf("empty env should fall back to vi, got %q", got)
	}
	if got := resolveEditor(func(k string) string {
		if k == "EDITOR" {
			return "nano"
		}
		return ""
	}); got != "nano" {
		t.Fatalf("EDITOR should be used, got %q", got)
	}
	if got := resolveEditor(func(k string) string {
		switch k {
		case "VISUAL":
			return "code -w"
		case "EDITOR":
			return "nano"
		}
		return ""
	}); got != "code -w" {
		t.Fatalf("VISUAL should take precedence, got %q", got)
	}
}

func TestTuiRequiresTTY(t *testing.T) {
	dir := writeFindingProject(t)
	var so, se bytes.Buffer
	// stdout is a buffer (non-TTY): tui must refuse, not hang.
	code := run([]string{"--project", dir, "tui"}, &so, &se, noEnv, noLookup)
	if code == 0 {
		t.Fatalf("tui on a non-tty should exit non-zero; stdout=%s", so.String())
	}
	if !strings.Contains(se.String()+so.String(), "terminal") {
		t.Fatalf("expected a 'requires a terminal' message, got: %s / %s", so.String(), se.String())
	}
}
