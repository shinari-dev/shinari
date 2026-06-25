// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shinari-dev/shinari/core/engine"
)

// EventMsg delivers one engine event into the program.
type EventMsg struct{ Event engine.Event }

// DoneMsg signals the run finished, carrying its result.
type DoneMsg struct {
	Res engine.RunResult
	Err error
}

type mode int

const (
	live mode = iota
	replay
)

// Model is the Bubble Tea model: it accumulates events and renders their
// reduction. In replay mode a cursor scrubs over the accumulated events.
type Model struct {
	mode   mode
	events []engine.Event
	cursor int
	width  int
	done   bool

	Res engine.RunResult
	Err error
}

// NewLive builds a model that folds events as they arrive.
func NewLive() Model { return Model{mode: live} }

// NewReplay builds a model over a fixed event slice, cursor at the end.
func NewReplay(events []engine.Event) Model {
	return Model{mode: replay, events: events, cursor: len(events)}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case EventMsg:
		m.events = append(m.events, msg.Event)
		m.cursor = len(m.events)
		return m, nil
	case DoneMsg:
		m.Res, m.Err, m.done = msg.Res, msg.Err, true
		if m.mode == live {
			return m, tea.Quit
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "left", "h":
			if m.mode == replay && m.cursor > 0 {
				m.cursor--
			}
		case "right", "l":
			if m.mode == replay && m.cursor < len(m.events) {
				m.cursor++
			}
		case "home", "g":
			if m.mode == replay {
				m.cursor = 0
			}
		case "end", "G":
			if m.mode == replay {
				m.cursor = len(m.events)
			}
		}
		return m, nil
	}
	return m, nil
}

func (m Model) View() string {
	run := engine.Reduce(m.events[:m.cursor])
	var header string
	switch {
	case m.mode == replay:
		header = fmt.Sprintf("shinari — replay  [%d/%d]  ←/→ scrub · q quit", m.cursor, len(m.events))
	case m.done:
		header = "shinari — " + string(m.Res.Verdict())
	default:
		header = "shinari — running…  q quit"
	}
	return RenderRun(run, header, m.width)
}
