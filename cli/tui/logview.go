// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/shinari-dev/shinari/core/engine"
)

// logKeys are the payload fields surfaced in the log, in priority order; the
// rest are dropped to keep each line readable.
var logKeys = []string{"verdict", "error", "reason", "value", "observed", "narrative", "detail"}

// RenderLog turns the raw event stream into one display line per event, each
// prefixed with a right-aligned line-number gutter.
func RenderLog(events []engine.Event) []string {
	gutterS := lipgloss.NewStyle().Foreground(warn) // amber line-number gutter
	timeS := lipgloss.NewStyle().Foreground(steel)
	typeS := lipgloss.NewStyle().Foreground(fgDim)
	textS := lipgloss.NewStyle().Foreground(fgSoft)
	failS := lipgloss.NewStyle().Foreground(fail)

	gutterW := len(strconv.Itoa(len(events))) // widest number sets the gutter width

	lines := make([]string, 0, len(events))
	for i, e := range events {
		target := e.Step
		if e.Verb != "" {
			if target != "" {
				target += "  "
			}
			target += e.Verb
		}
		detail := logDetail(e.Payload)
		style := textS
		if e.Type == engine.EvStepFailed {
			style = failS
		}
		gutter := gutterS.Render(fmt.Sprintf("%*d", gutterW, i+1))
		lines = append(lines,
			gutter+"  "+
				timeS.Render(e.Time.Format("15:04:05"))+"  "+
				typeS.Render(string(e.Type))+"  "+
				style.Render(strings.TrimSpace(target+"  "+detail)))
	}
	return lines
}

func logDetail(p map[string]any) string {
	if p == nil {
		return ""
	}
	var parts []string
	for _, k := range logKeys { // fixed priority order
		if v, ok := p[k]; ok && v != nil && v != "" {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	return strings.Join(parts, " ")
}
