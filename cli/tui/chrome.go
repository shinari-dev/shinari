// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderStatusBar is the top bar: ● <label> (e.g. "shinari - 0.3.0-dev"),
// one full-width row.
func renderStatusBar(label string, width int) string {
	dot := lipgloss.NewStyle().Foreground(ember).Bold(true).Render("●")
	word := lipgloss.NewStyle().Foreground(fg).Bold(true).Render(label)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(fg).
		Width(width - 2).Render(dot + " " + word)
}

func renderTabs(active string) string {
	on := lipgloss.NewStyle().Foreground(ember).Bold(true).Underline(true)
	off := lipgloss.NewStyle().Foreground(fgDim)
	mark := func(s string) string {
		if s == active {
			return on.Render(s)
		}
		return off.Render(s)
	}
	sep := lipgloss.NewStyle().Foreground(borderDim).Render("  │  ")
	return mark("Project") + sep + mark("Scenarios")
}

// keyHint renders a visible shortcut bar: bright ember keys, soft descriptions.
// Each pair is {key, description}, e.g. {"↵", "edit"}.
func keyHint(pairs ...[2]string) string {
	keyS := lipgloss.NewStyle().Foreground(ember).Bold(true)
	descS := lipgloss.NewStyle().Foreground(fgSoft)
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, keyS.Render(p[0])+" "+descS.Render(p[1]))
	}
	return strings.Join(parts, lipgloss.NewStyle().Foreground(borderDim).Render("   "))
}
