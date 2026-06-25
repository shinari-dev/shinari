// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/shinari-dev/shinari/core/engine"
)

// Palette — sourced from docs/assets/css/main.css + the logo SVG.
var (
	canvas    = lipgloss.Color("#0a0b0e") // website --bg; applied to the terminal via OSC 11
	ember     = lipgloss.Color("#ff4f2b")
	borderDim = lipgloss.Color("#2c2f37")
	fg        = lipgloss.Color("#eceae6")
	fgSoft    = lipgloss.Color("#b4b7bd")
	fgDim     = lipgloss.Color("#82868e")
	pass      = lipgloss.Color("#3ecf8e")
	warn      = lipgloss.Color("#ffb454")
	steel     = lipgloss.Color("#62b3f0")
	fail      = lipgloss.Color("#ff5c57")
)

func badge(label string, fgc, bgc lipgloss.Color) string {
	return lipgloss.NewStyle().Foreground(fgc).Background(bgc).Bold(true).Render(" " + label + " ")
}

// verdictBadge maps a scenario/finding verdict string to a colored chip.
func verdictBadge(v string) string {
	switch v {
	case string(engine.ScenarioPassed):
		return badge(v, canvas, pass)
	case string(engine.ScenarioFailed), string(engine.ScenarioErrored):
		return badge(v, canvas, fail)
	case "FINDING", "NOW PASSES", "RUNNING":
		return badge(v, canvas, ember)
	default:
		return lipgloss.NewStyle().Foreground(fgSoft).Render(v)
	}
}

func glyph(v engine.CheckVerdict) string {
	switch v {
	case engine.CheckPass:
		return lipgloss.NewStyle().Foreground(pass).Render("✓")
	case engine.CheckFail:
		return lipgloss.NewStyle().Foreground(fail).Bold(true).Render("✗")
	case engine.CheckFinding:
		return lipgloss.NewStyle().Foreground(ember).Render("●")
	case engine.CheckSkip:
		return lipgloss.NewStyle().Foreground(fgDim).Render("–")
	default:
		return " "
	}
}

// panelStyle is a rounded border sized w×h; ember when focused, white otherwise.
func panelStyle(w, h int, focused bool) lipgloss.Style {
	bc := fg
	if focused {
		bc = ember
	}
	s := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(bc).Foreground(fg)
	if w > 2 {
		s = s.Width(w - 2)
	}
	if h > 2 {
		s = s.Height(h - 2)
	}
	return s
}

// framedPane draws content in a rounded w×h box whose top border carries a
// right-aligned title chip in the pane's accent hue, like a labeled fieldset.
// Focus drives brightness: the focused pane gets a filled bright chip and an
// accent border; an unfocused one a muted accent-on-slate chip and dim border.
func framedPane(w, h int, title string, focused bool, accent lipgloss.Color, content string) string {
	bc := fgDim
	chip := lipgloss.NewStyle().Foreground(accent).Background(borderDim).Bold(true).Render(" " + title + " ")
	if focused {
		bc, chip = accent, badge(title, canvas, accent)
	}
	border := lipgloss.NewStyle().Foreground(bc)
	inner := w - 2 // between the corner runes
	if inner < 0 {
		inner = 0
	}
	fill := inner - 1 - lipgloss.Width(chip) // chip sits at the right, one dash after it
	if fill < 0 {
		fill = 0
	}
	top := border.Render("╭"+strings.Repeat("─", fill)) + chip + border.Render("─╮")
	bottom := border.Render("╰" + strings.Repeat("─", inner) + "╯")

	body := lipgloss.NewStyle().Width(inner).Height(h - 2).MaxHeight(h - 2).Render(content)
	side := border.Render("│")
	rows := strings.Split(body, "\n")
	for i, row := range rows {
		rows[i] = side + row + side
	}
	return strings.Join(append(append([]string{top}, rows...), bottom), "\n")
}
