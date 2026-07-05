// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/shinari-dev/shinari/core/engine"
)

// scenarioFrac is the share of the run area's height given to the scenarios
// (summary) pane; the rest goes to the log firehose.
const scenarioFrac = 40

type runModel struct {
	events   []engine.Event
	done     bool
	res      engine.RunResult
	err      error
	cancel   context.CancelFunc
	afterRan bool
	afterErr error // reports/history write failure — shown instead of "recorded ✓"
	dry      bool  // dry-run (actions skipped, not recorded)

	// The run view is two stacked, independently scrolling frames managed by the
	// shared twoPane: the scenario summary on top, the streamed event log below
	// (tail-follows). ⇥ moves focus, f fullscreens the focused pane, ↑↓ scroll it.
	tp     twoPane
	scVp   viewport.Model // top: scenario summary
	logVp  viewport.Model // bottom: streamed event log
	width  int
	height int

	// AfterRun runs once when the run completes: records history + writes
	// reports. Injected by the command so cli/tui stays decoupled. Its error
	// is surfaced in the header — a full disk must not read as "recorded ✓".
	AfterRun func(engine.RunResult, []engine.Event) error
}

func newRun() runModel {
	tp := newTwoPane(paneSpec{"Run", steel}, paneSpec{"Logs", ember}, scenarioFrac)
	tp.focus = 1 // the log firehose is focused by default
	return runModel{tp: tp, scVp: viewport.New(0, 0), logVp: viewport.New(0, 0)}
}

// setSize fits the two panes to the area below the verdict header and reflows.
func (r *runModel) setSize(w, h int) {
	r.width, r.height = w, h
	r.tp.setSize(w, h-lipgloss.Height(r.headerLine()))
	topH, bottomH := r.tp.paneHeights()
	r.scVp.Width, r.scVp.Height = w-2, paneContentHeight(topH)
	r.logVp.Width, r.logVp.Height = w-2, paneContentHeight(bottomH)
	r.refresh()
}

// refresh re-renders both panes. The log pane stays pinned to the tail while the
// user hasn't scrolled away (live runs read like a log follow); the summary keeps
// its position so the verdicts don't jump.
func (r *runModel) refresh() {
	r.scVp.SetContent(r.summaryContent())
	follow := r.logVp.AtBottom()
	r.logVp.SetContent(r.logContent())
	if follow {
		r.logVp.GotoBottom()
	}
}

func (r runModel) Update(msg tea.Msg) (runModel, tea.Cmd) {
	switch msg := msg.(type) {
	case EventMsg:
		r.events = append(r.events, msg.Event)
		r.refresh()
		return r, nil
	case DoneMsg:
		r.done, r.res, r.err = true, msg.Res, msg.Err
		if !r.afterRan && r.AfterRun != nil {
			r.afterErr = r.AfterRun(msg.Res, r.events)
			r.afterRan = true
		}
		r.refresh()
		return r, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "f":
			r.tp.toggleFull()
			r.setSize(r.width, r.height)
			return r, nil
		case "tab":
			r.tp.toggleFocus()
			r.setSize(r.width, r.height) // fullscreen reveals the now-focused pane
			return r, nil
		}
		var cmd tea.Cmd
		if r.tp.bottomFocused() { // scroll the focused pane
			r.logVp, cmd = r.logVp.Update(msg)
		} else {
			r.scVp, cmd = r.scVp.Update(msg)
		}
		return r, cmd
	}
	return r, nil
}

func (r runModel) View() string {
	return lipgloss.JoinVertical(lipgloss.Left, r.headerLine(), r.tp.render(r.scVp.View(), r.logVp.View()))
}

// headerLine is the verdict badge shown above the panes (RUNNING while live).
func (r runModel) headerLine() string {
	if r.done {
		h := verdictBadge(string(r.res.Verdict()))
		if r.afterRan {
			if r.afterErr != nil {
				h += lipgloss.NewStyle().Foreground(fail).Render("  ·  record failed: " + r.afterErr.Error())
			} else {
				h += lipgloss.NewStyle().Foreground(fgDim).Render("  ·  recorded ✓")
			}
		}
		return h
	}
	return verdictBadge("RUNNING")
}

func (r runModel) summaryContent() string {
	label := "run · "
	if r.dry {
		label = "dry-run · "
	}
	return RenderRun(engine.Reduce(r.events), label+r.scenarioName(), r.scVp.Width)
}

func (r runModel) logContent() string { return strings.Join(RenderLog(r.events), "\n") }

func (r runModel) scenarioName() string {
	scs := r.res.Scenarios
	if !r.done {
		scs = engine.Reduce(r.events).Scenarios
	}
	switch len(scs) {
	case 0:
		return ""
	case 1:
		return scs[0].Name
	default:
		return fmt.Sprintf("%d scenarios", len(scs))
	}
}
