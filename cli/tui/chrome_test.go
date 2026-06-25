// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"
	"testing"
)

func TestStatusBarShowsLabel(t *testing.T) {
	out := renderStatusBar("shinari - 0.3.0-dev", 80)
	for _, want := range []string{"●", "shinari - 0.3.0-dev"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status bar missing %q in:\n%s", want, out)
		}
	}
}
