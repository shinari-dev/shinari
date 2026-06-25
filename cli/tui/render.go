// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package tui is the terminal UI at the CLI edge: a Bubble Tea front end whose
// view is the engine's event-stream reduction rendered. It consumes the
// engine's Emitter/Event/Reduce; core never imports it.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/shinari-dev/shinari/core/engine"
)

// RenderRun renders a reduced run: a header, then each scenario with its steps
// and findings. It is pure: same input, same output, no terminal required.
func RenderRun(run engine.RunResult, header string, width int) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ember).Render(header))
	b.WriteString("\n\n")
	if len(run.Scenarios) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(fgDim).Render("  (waiting for first scenario…)"))
		b.WriteString("\n")
		return b.String()
	}
	for _, sc := range run.Scenarios {
		line := sc.Name
		if sc.Verdict != "" {
			line = sc.Name + "  " + verdictBadge(string(sc.Verdict))
		}
		b.WriteString(line + "\n")
		for _, st := range sc.Steps {
			b.WriteString("  " + glyph(st.Verdict) + " " + st.Label() + "\n")
		}
		for _, f := range sc.Findings {
			tag := "FINDING"
			if f.NowPasses {
				tag = "NOW PASSES"
			}
			b.WriteString("  " + lipgloss.NewStyle().Foreground(ember).Render("● "+tag) + " " +
				lipgloss.NewStyle().Foreground(fgDim).Render("["+f.ID+"]") + " " + f.Narrative + "\n")
		}
	}
	return b.String()
}
