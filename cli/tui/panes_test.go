// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"
	"testing"
)

func TestTwoPaneFullscreenGivesFocusedAll(t *testing.T) {
	tp := newTwoPane(paneSpec{"Top", steel}, paneSpec{"Bottom", ember}, 40)
	tp.setSize(80, 20)
	tp.toggleFull()
	tp.toggleFocus() // focus bottom
	top, bottom := tp.paneHeights()
	if top != 0 || bottom != 20 {
		t.Fatalf("fullscreen+focus bottom: want top=0 bottom=20, got %d/%d", top, bottom)
	}
}

func TestTwoPaneRenderFramesBoth(t *testing.T) {
	tp := newTwoPane(paneSpec{"Alpha", steel}, paneSpec{"Beta", ember}, 50)
	tp.setSize(40, 12)
	out := stripANSI(tp.render("topcontent", "botcontent"))
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Fatalf("both pane titles should render:\n%s", out)
	}
	if !strings.Contains(out, "topcontent") || !strings.Contains(out, "botcontent") {
		t.Fatalf("both contents should render:\n%s", out)
	}
}

func TestTwoPaneRenderFullscreenHidesOther(t *testing.T) {
	tp := newTwoPane(paneSpec{"Alpha", steel}, paneSpec{"Beta", ember}, 50)
	tp.setSize(40, 12)
	tp.toggleFull() // focus top (default), fullscreen
	out := stripANSI(tp.render("topcontent", "botcontent"))
	if !strings.Contains(out, "topcontent") {
		t.Fatalf("fullscreen focused (top) content should show:\n%s", out)
	}
	if strings.Contains(out, "botcontent") || strings.Contains(out, "Beta") {
		t.Fatalf("fullscreen should hide the unfocused pane:\n%s", out)
	}
}
