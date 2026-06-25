// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import "github.com/charmbracelet/lipgloss"

// renderLogo draws the favicon essence: the unbroken line deflecting around the
// ember fault dot, then the wordmark and tagline, centered to width.
func renderLogo(width int) string {
	line := lipgloss.NewStyle().Foreground(fg)
	dot := lipgloss.NewStyle().Foreground(ember).Bold(true)
	// Art rows share one width so JoinVertical(Center) keeps the box-drawing connected.
	block := lipgloss.JoinVertical(lipgloss.Center,
		dot.Render("        ●        "),
		line.Render("╶─────╮   ╭─────╴"),
		line.Render("      ╰───╯      "),
		"",
		lipgloss.NewStyle().Foreground(ember).Bold(true).Render("s h i n a r i"),
		lipgloss.NewStyle().Foreground(fgDim).Render("resilience integration testing"),
		"",
		lipgloss.NewStyle().Foreground(fgDim).Render("press any key to continue"),
	)
	if width < 1 {
		width = lipgloss.Width(block)
	}
	return lipgloss.Place(width, lipgloss.Height(block)+2, lipgloss.Center, lipgloss.Center, block)
}
