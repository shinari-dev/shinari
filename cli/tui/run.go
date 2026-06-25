// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shinari-dev/shinari/core/engine"
)

// RunApp runs the interactive control center to completion. It sets the package
// program handle so a run's goroutine can stream EventMsgs via Send.
func RunApp(app App) error {
	// Paint the terminal background to the brand canvas (OSC 11; ignored where
	// unsupported, restored on exit).
	fmt.Fprint(os.Stdout, "\x1b]11;#0a0b0e\a")
	defer fmt.Fprint(os.Stdout, "\x1b]111\a")
	p := tea.NewProgram(app, tea.WithAltScreen())
	program = p
	_, err := p.Run()
	return err
}

// RunReplay shows an interactive replay of a saved event slice.
func RunReplay(events []engine.Event) error {
	_, err := tea.NewProgram(NewReplay(events)).Run()
	return err
}
