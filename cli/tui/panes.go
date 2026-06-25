// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import "github.com/charmbracelet/lipgloss"

// paneSpec is the static chrome of one pane in a twoPane: its border title and
// accent hue.
type paneSpec struct {
	title  string
	accent lipgloss.Color
}

// twoPane is the shared stacked-pane layout behind the run view and the
// Scenarios screen: two framed panes, ⇥ focus, f fullscreen. It owns layout,
// focus, fullscreen, and the framed chrome; callers own each pane's content and
// the routing of scroll/nav keys to the focused pane.
type twoPane struct {
	top, bottom paneSpec
	focus       int  // 0 = top, 1 = bottom
	full        bool // fullscreen the focused pane (hide the other)
	split       int  // top pane's % of the height when not fullscreen
	width       int
	height      int
}

func newTwoPane(top, bottom paneSpec, split int) twoPane {
	return twoPane{top: top, bottom: bottom, split: split}
}

func (t twoPane) topFocused() bool    { return t.focus == 0 }
func (t twoPane) bottomFocused() bool { return t.focus == 1 }
func (t *twoPane) toggleFocus()       { t.focus = 1 - t.focus }
func (t *twoPane) toggleFull()        { t.full = !t.full }
func (t *twoPane) setSize(w, h int)   { t.width, t.height = w, h }

// paneHeights is the outer height of each pane. When fullscreen, the focused
// pane gets the whole height and the other gets zero.
func (t twoPane) paneHeights() (topH, bottomH int) {
	if t.full {
		if t.focus == 0 {
			return t.height, 0
		}
		return 0, t.height
	}
	topH = t.height * t.split / 100
	return topH, t.height - topH
}

// paneContentHeight is the height available inside a pane of the given outer
// height, after framedPane's rounded border.
func paneContentHeight(outer int) int {
	if outer < 3 {
		return 0
	}
	return outer - 2
}

// render frames the two pre-sized contents. The focused pane's border lights up;
// in fullscreen only the focused pane is drawn.
func (t twoPane) render(topContent, bottomContent string) string {
	topH, bottomH := t.paneHeights()
	if t.full {
		if t.focus == 0 {
			return framedPane(t.width, topH, t.top.title, true, t.top.accent, topContent)
		}
		return framedPane(t.width, bottomH, t.bottom.title, true, t.bottom.accent, bottomContent)
	}
	top := framedPane(t.width, topH, t.top.title, t.focus == 0, t.top.accent, topContent)
	bottom := framedPane(t.width, bottomH, t.bottom.title, t.focus == 1, t.bottom.accent, bottomContent)
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}
