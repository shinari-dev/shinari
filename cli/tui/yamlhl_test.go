// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestHighlightYAMLPreservesTextAndColors(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	src := "# a comment\nname: cache-outage\nsetup:\n  - run: exec.run\n    with: \"true\"\ncount: 10\n"
	out := highlightYAML(src)

	// every original token survives (highlighting only adds styling).
	for _, want := range []string{"a comment", "name", "cache-outage", "run", "exec.run", "10"} {
		if !strings.Contains(out, want) {
			t.Fatalf("highlight dropped %q:\n%s", want, out)
		}
	}
	// it actually colored something (ANSI escapes present, output longer than input).
	if !strings.Contains(out, "\x1b[") || len(out) <= len(src) {
		t.Fatalf("expected ANSI-colored output, got:\n%q", out)
	}
	// line count is unchanged (no reflow).
	if got, want := strings.Count(out, "\n"), strings.Count(src, "\n"); got != want {
		t.Fatalf("line count changed: got %d want %d", got, want)
	}
}
