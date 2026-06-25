// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Tab        key.Binding
	ShiftTab   key.Binding
	Fullscreen key.Binding
	DryRun     key.Binding
	Run        key.Binding
	RunAll     key.Binding
	Edit       key.Binding
	EditPrj    key.Binding
	New        key.Binding
	Delete     key.Binding
	Cancel     key.Binding
	Back       key.Binding
	Quit       key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Tab:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "focus")),
		ShiftTab:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "screen")),
		Fullscreen: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "full")),
		DryRun:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "dry-run")),
		Run:        key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run")),
		RunAll:     key.NewBinding(key.WithKeys("R"), key.WithHelp("⇧r", "run all")),
		Edit:       key.NewBinding(key.WithKeys("E"), key.WithHelp("⇧e", "edit")),
		EditPrj:    key.NewBinding(key.WithKeys("P"), key.WithHelp("⇧p", "project")),
		New:        key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		Delete:     key.NewBinding(key.WithKeys("D"), key.WithHelp("⇧D", "del")),
		Cancel:     key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "cancel")),
		Back:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}
