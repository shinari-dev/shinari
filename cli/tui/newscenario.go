// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// createdMsg reports the outcome of scaffolding a new scenario file.
type createdMsg struct {
	path string
	err  error
}

// newModel is the modal form for scaffolding a scenario.
type newModel struct {
	name  textinput.Model
	suite textinput.Model
	kind  string // "minimal" | "fault-inject"
	focus int    // 0 name, 1 suite
}

func newNewModel() newModel {
	n := textinput.New()
	n.Placeholder = "cache-outage"
	n.Focus()
	s := textinput.New()
	s.Placeholder = "resilience"
	s.SetValue("resilience")
	return newModel{name: n, suite: s, kind: "minimal"}
}

func (m newModel) Update(msg tea.Msg) (newModel, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "tab":
			m.focus = (m.focus + 1) % 2
			if m.focus == 0 {
				m.name.Focus()
				m.suite.Blur()
			} else {
				m.suite.Focus()
				m.name.Blur()
			}
			return m, nil
		case "ctrl+t":
			if m.kind == "minimal" {
				m.kind = "fault-inject"
			} else {
				m.kind = "minimal"
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	if m.focus == 0 {
		m.name, cmd = m.name.Update(msg)
	} else {
		m.suite, cmd = m.suite.Update(msg)
	}
	return m, cmd
}

// submit returns a command that writes the scaffolded file under root.
func (m newModel) submit(root string) tea.Cmd {
	name, suite, kind := m.name.Value(), m.suite.Value(), m.kind
	return func() tea.Msg {
		path, err := writeScenario(root, suite, name, kind)
		return createdMsg{path: path, err: err}
	}
}

func (m newModel) View() string {
	sel := func(k string) string {
		if k == m.kind {
			return lipgloss.NewStyle().Foreground(ember).Render("◉ " + k)
		}
		return lipgloss.NewStyle().Foreground(fgDim).Render("○ " + k)
	}
	rows := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Foreground(ember).Bold(true).Render("new scenario"),
		"name   "+m.name.View(),
		"suite  "+m.suite.View(),
		"template  "+sel("minimal")+"  "+sel("fault-inject")+lipgloss.NewStyle().Foreground(fgDim).Render("  (ctrl+t)"),
		lipgloss.NewStyle().Foreground(fgDim).Render("↵ create   esc cancel"),
	)
	return panelStyle(50, 9, true).Render(rows)
}
