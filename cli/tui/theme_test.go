// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestFramedPaneTitleOnBorder(t *testing.T) {
	out := framedPane(30, 4, "logs", true, ember, "hello")
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("want 4 lines (top + 2 body + bottom), got %d:\n%s", len(lines), out)
	}
	top := stripANSI(lines[0])
	if !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") {
		t.Fatalf("top line should be a rounded border, got %q", top)
	}
	if !strings.Contains(top, "logs") {
		t.Fatalf("the title should sit on the top border, got %q", top)
	}
	if bottom := stripANSI(lines[3]); !strings.HasPrefix(bottom, "╰") || !strings.HasSuffix(bottom, "╯") {
		t.Fatalf("bottom line should close the border, got %q", bottom)
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w != 30 {
			t.Fatalf("line %d width want 30, got %d: %q", i, w, stripANSI(l))
		}
	}
}

func TestFramedPaneFocusChangesStyle(t *testing.T) {
	// The renderer strips color off a non-TTY by default; force a profile so the
	// focus-driven ember/slate difference is observable.
	lipgloss.SetColorProfile(termenv.TrueColor)
	on := framedPane(30, 3, "logs", true, ember, "x")
	off := framedPane(30, 3, "logs", false, ember, "x")
	if on == off {
		t.Fatal("focused and unfocused panes should render differently (border/title color)")
	}
}
