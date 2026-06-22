// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"strings"
	"testing"
)

func TestPlainPaletteIsNoOp(t *testing.T) {
	var p Palette // zero value: disabled
	if got := p.Pass("ok"); got != "ok" {
		t.Errorf("plain palette wrapped text: %q", got)
	}
	if got := NewPalette(false).Fail("boom"); got != "boom" {
		t.Errorf("disabled palette wrapped text: %q", got)
	}
}

func TestColorPaletteWrapsInAnsi(t *testing.T) {
	p := NewPalette(true)
	got := p.Finding("gap")
	if !strings.HasPrefix(got, "\x1b[") || !strings.HasSuffix(got, "\x1b[0m") || !strings.Contains(got, "gap") {
		t.Errorf("expected ANSI-wrapped text, got %q", got)
	}
	// Empty strings are never wrapped, so blank padding stays blank.
	if got := p.Pass(""); got != "" {
		t.Errorf("empty string wrapped: %q", got)
	}
}

func TestVerdictColorsByOutcome(t *testing.T) {
	p := NewPalette(true)
	if p.Verdict("PASSED") == "PASSED" {
		t.Error("PASSED was not colored")
	}
	if got := NewPalette(false).Verdict("FAILED"); got != "FAILED" {
		t.Errorf("disabled verdict colored: %q", got)
	}
}
