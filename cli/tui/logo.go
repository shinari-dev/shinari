// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import "github.com/charmbracelet/lipgloss"

// renderLogo draws the favicon essence: the unbroken line deflecting around the
// ember fault dot, then the wordmark and the site's hero line (kicker + title),
// centered to width. The title's closing period is ember, echoing the vermilion
// dot on the website hero.
func renderLogo(width int) string {
	line := lipgloss.NewStyle().Foreground(fg)
	dot := lipgloss.NewStyle().Foreground(ember).Bold(true)
	title := lipgloss.NewStyle().Foreground(fg).Bold(true).Render("prove your system survives failure") +
		dot.Render(".")
	// Art rows share one width so JoinVertical(Center) keeps the box-drawing connected.
	block := lipgloss.JoinVertical(lipgloss.Center,
		dot.Render("        ●        "),
		line.Render("╶─────╮   ╭─────╴"),
		line.Render("      ╰───╯      "),
		"",
		lipgloss.NewStyle().Foreground(ember).Bold(true).Render("s h i n a r i"),
		"",
		lipgloss.NewStyle().Foreground(fgDim).Render("// deterministic fault injection"),
		title,
		"",
		lipgloss.NewStyle().Foreground(fgDim).Render("press any key to continue"),
	)
	if width < 1 {
		width = lipgloss.Width(block)
	}
	return lipgloss.Place(width, lipgloss.Height(block)+2, lipgloss.Center, lipgloss.Center, block)
}
