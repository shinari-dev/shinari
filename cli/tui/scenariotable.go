// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/shinari-dev/shinari/cli/history"
	"github.com/shinari-dev/shinari/core/model"
)

// selBg is the background of the selected table row.
var selBg = borderDim

// scenarioItem adapts a scenario to a bubbles/list item. FilterValue spans the
// name and tags so `/` search matches either.
type scenarioItem struct{ sc *model.Scenario }

func (i scenarioItem) Title() string       { return i.sc.Name }
func (i scenarioItem) Description() string { return i.sc.Description }
func (i scenarioItem) FilterValue() string { return i.sc.Name + " " + strings.Join(i.sc.Tags, " ") }

// relAge renders a duration as a compact relative age (now / 5m / 3h / 2d).
func relAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// colWidths splits the inner pane width into name / tags / status / last
// columns. Fixed marker (2) + three single-space gaps (3) = 5 cols of chrome.
func colWidths(inner int) (nameW, tagsW, statusW, lastW int) {
	statusW = 12 // widest verdict text ("INCONCLUSIVE")
	lastW = 5    // "now" / "12h" / "30d"
	nameW = 34   // scenario names are short; tags take the slack
	if max := inner - 5 - statusW - lastW - 6; nameW > max {
		nameW = max
	}
	if nameW < 8 {
		nameW = 8
	}
	tagsW = inner - 5 - nameW - statusW - lastW
	if tagsW < 6 {
		tagsW = 6
	}
	return
}

func statusColor(v string) lipgloss.Color {
	switch v {
	case "PASSED":
		return pass
	case "FAILED", "ERRORED":
		return fail
	case "INCONCLUSIVE":
		return warn
	case "":
		return fgDim
	default:
		return fgSoft
	}
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// cell renders text in a fixed-width column on a single line: padded when short,
// hard-clipped when long. Inline(true) is essential — without it lipgloss wraps
// an over-long cell onto a second line, which a Height()==1 delegate cannot show.
func cell(text string, width int, style lipgloss.Style) string {
	return style.Width(width).MaxWidth(width).Inline(true).Render(text)
}

// renderScenarioRow renders one table row: marker, name, tags, status, last.
// A selected row is a continuous highlighted bar — every segment (marker, cells,
// gaps) carries the selection background so there are no gaps in the highlight.
func renderScenarioRow(name string, tags []string, status, last string, selected bool, inner int) string {
	nameW, tagsW, statusW, lastW := colWidths(inner)
	var bg lipgloss.TerminalColor = lipgloss.NoColor{}
	nameFg := fgSoft
	if selected {
		bg, nameFg = selBg, fg
	}
	st := func(fg lipgloss.TerminalColor) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(fg).Background(bg)
	}
	mk := "  "
	if selected {
		mk = "› "
	}
	marker := st(ember).Render(mk)
	gap := st(fgDim).Render(" ")
	nameCell := cell(name, nameW, st(nameFg).Bold(selected))
	tagCell := cell(strings.Join(tags, " "), tagsW, st(steel))
	statusCell := cell(orDash(status), statusW, st(statusColor(status)))
	lastCell := cell(orDash(last), lastW, st(fgDim))
	return marker + nameCell + gap + tagCell + gap + statusCell + gap + lastCell
}

// scenarioHeader is the static column-title row above the table rows.
func scenarioHeader(inner int) string {
	nameW, tagsW, statusW, lastW := colWidths(inner)
	h := lipgloss.NewStyle().Foreground(fgDim).Bold(true)
	return "  " + cell("NAME", nameW, h) + " " + cell("TAGS", tagsW, h) + " " +
		cell("STATUS", statusW, h) + " " + cell("LAST", lastW, h)
}

// scenarioDelegate renders scenarioItems as one-line table rows.
type scenarioDelegate struct {
	recs  []history.Record
	inner int
}

func (d scenarioDelegate) Height() int                         { return 1 }
func (d scenarioDelegate) Spacing() int                        { return 0 }
func (d scenarioDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d scenarioDelegate) Render(w io.Writer, m list.Model, index int, it list.Item) {
	si, ok := it.(scenarioItem)
	if !ok {
		return
	}
	status, last := "", ""
	if runs := history.RunsFor(d.recs, si.sc.Name); len(runs) > 0 {
		status = runs[0].Verdict
		last = relAge(time.Since(runs[0].Time))
	}
	_, _ = io.WriteString(w, renderScenarioRow(si.sc.Name, si.sc.Tags, status, last, index == m.Index(), d.inner))
}
