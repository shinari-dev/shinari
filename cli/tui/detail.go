// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/shinari-dev/shinari/cli/history"
	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/model"
)

// lastRunLine summarizes the newest run of a scenario: verdict (colored) ·
// relative age · duration. Returns "—" when it has never run.
func lastRunLine(recs []history.Record, name string, now time.Time) string {
	runs := history.RunsFor(recs, name)
	if len(runs) == 0 {
		return lipgloss.NewStyle().Foreground(fgDim).Render("—")
	}
	r := runs[0]
	meta := relAge(now.Sub(r.Time)) + " ago"
	if r.Duration > 0 {
		meta += "  ·  " + r.Duration.Round(100*time.Millisecond).String()
	}
	return lipgloss.NewStyle().Foreground(statusColor(r.Verdict)).Bold(true).Render(r.Verdict) +
		lipgloss.NewStyle().Foreground(fgDim).Render("  ·  "+meta)
}

// scenarioShape is a one-line structural summary of a scenario: step count plus
// fault and finding counts when present.
func scenarioShape(sc *model.Scenario) string {
	steps, faults, findings := 0, 0, 0
	for _, sec := range sc.Sections() {
		for _, st := range sec.Steps {
			steps++
			if st.Effect != "" && st.Effect != "none" {
				faults++
			}
			if st.Finding != "" {
				findings++
			}
		}
	}
	parts := []string{fmt.Sprintf("%d steps", steps)}
	if faults > 0 {
		parts = append(parts, fmt.Sprintf("%d faults", faults))
	}
	if findings > 0 {
		parts = append(parts, fmt.Sprintf("%d findings", findings))
	}
	return strings.Join(parts, "  ·  ")
}

type subView int

const (
	subOverview subView = iota
	subExplain
	subSource
	subRuns
	subCount
)

func (s subView) label() string {
	switch s {
	case subExplain:
		return "Explain"
	case subSource:
		return "View"
	case subRuns:
		return "History"
	default:
		return "Overview"
	}
}

type detailModel struct {
	set       *discover.Set
	scenario  *model.Scenario
	sub       subView
	vp        viewport.Model
	width     int
	height    int
	runs      []history.Record // History sub-tab: this scenario's runs, newest first
	runCursor int              // selected run in the History sub-tab

	// injected by the command so cli/tui stays decoupled from package main.
	ExplainFn func(*model.Scenario) string
}

func newDetail(set *discover.Set) detailModel {
	return detailModel{set: set, sub: subOverview, vp: viewport.New(0, 0)}
}

func (d *detailModel) setSize(w, h int) {
	d.width, d.height = w, h
	d.vp.Width = max(0, w-2)  // inside the framed pane's border
	d.vp.Height = max(0, h-4) // border (2) + sub-tab line + blank (2)
}

// Update cycles sub-views with ←/→. In the History sub-tab ↑↓ move the run
// selection; everywhere else they scroll the content.
func (d detailModel) Update(msg tea.Msg) (detailModel, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "right":
			return d.gotoSub((d.sub + 1) % subCount), nil
		case "left":
			return d.gotoSub((d.sub + subCount - 1) % subCount), nil
		case "up", "k":
			if d.sub == subRuns {
				if d.runCursor > 0 {
					d.runCursor--
				}
				d.vp.SetContent(d.runsContent())
				return d, nil
			}
		case "down", "j":
			if d.sub == subRuns {
				if d.runCursor < len(d.runs)-1 {
					d.runCursor++
				}
				d.vp.SetContent(d.runsContent())
				return d, nil
			}
		}
	}
	var cmd tea.Cmd
	d.vp, cmd = d.vp.Update(msg)
	return d, cmd
}

// gotoSub switches to sub s and loads its content into the viewport.
func (d detailModel) gotoSub(s subView) detailModel {
	d.sub = s
	switch s {
	case subOverview:
		d.vp.SetContent(d.overview(d.scenario))
		d.vp.GotoTop()
	case subExplain:
		d.vp.SetContent(d.explainContent())
		d.vp.GotoTop()
	case subSource:
		d.vp.SetContent(d.sourceContent())
		d.vp.GotoTop()
	case subRuns:
		recs, _ := history.Load(history.Path(d.set.Root))
		if d.scenario != nil {
			d.runs = history.RunsFor(recs, d.scenario.Name)
		} else {
			d.runs = nil
		}
		d.runCursor = 0
		d.vp.SetContent(d.runsContent())
		d.vp.GotoTop()
	}
	return d
}

func (d detailModel) explainContent() string {
	if d.ExplainFn == nil || d.scenario == nil {
		return "(explain unavailable)"
	}
	return d.ExplainFn(d.scenario)
}

func (d detailModel) sourceContent() string {
	if d.scenario == nil || d.scenario.File == "" {
		return "(no source file)"
	}
	b, err := os.ReadFile(d.scenario.File)
	if err != nil {
		return "read error: " + err.Error()
	}
	return highlightYAML(string(b))
}

// runsContent renders the History sub-tab: a selectable list of this scenario's
// runs, then the detail (verdict · duration · findings) of the selected run.
func (d detailModel) runsContent() string {
	if d.scenario == nil {
		return ""
	}
	if len(d.runs) == 0 {
		return lipgloss.NewStyle().Foreground(fgDim).Render("(no recorded runs yet)")
	}
	dim := lipgloss.NewStyle().Foreground(fgDim)
	dur := func(r history.Record) string {
		if r.Duration > 0 {
			return r.Duration.Round(100 * time.Millisecond).String()
		}
		return ""
	}
	var b strings.Builder
	for i, r := range d.runs {
		marker, ts := "  ", dim.Render(r.Time.Format("2006-01-02 15:04"))
		if i == d.runCursor {
			marker = lipgloss.NewStyle().Foreground(ember).Render("› ")
			ts = lipgloss.NewStyle().Foreground(fg).Render(r.Time.Format("2006-01-02 15:04"))
		}
		v := lipgloss.NewStyle().Foreground(statusColor(r.Verdict)).Render(r.Verdict)
		fmt.Fprintf(&b, "%s%s  %s  %s\n", marker, ts, v, dim.Render(dur(r)))
	}

	r := d.runs[d.runCursor]
	b.WriteString("\n" + dim.Render("Run ") + lipgloss.NewStyle().Foreground(fg).Render(r.Time.Format("2006-01-02 15:04:05")) + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(statusColor(r.Verdict)).Bold(true).Render(r.Verdict))
	if s := dur(r); s != "" {
		b.WriteString(dim.Render("  ·  " + s))
	}
	b.WriteString("\n\n")
	var fs []history.Finding
	for _, f := range r.Findings {
		if f.Scenario == "" || f.Scenario == d.scenario.Name {
			fs = append(fs, f)
		}
	}
	if len(fs) == 0 {
		b.WriteString(dim.Render("(no findings recorded)"))
		return b.String()
	}
	b.WriteString(dim.Render("Findings") + "\n")
	for _, f := range fs {
		note := ""
		if f.NowPasses {
			note = dim.Render("  (now passes)")
		}
		fmt.Fprintf(&b, "%s %s %s%s\n",
			lipgloss.NewStyle().Foreground(ember).Render("●"),
			dim.Render("["+f.ID+"]"), f.Narrative, note)
	}
	return b.String()
}

// inner renders the detail content (sub-tab strip + scrollable body) without a
// frame; the caller wraps it via twoPane so chrome stays uniform.
func (d detailModel) inner(sc *model.Scenario) string {
	if sc == nil {
		return lipgloss.NewStyle().Foreground(fgDim).Render("no scenario selected")
	}
	return lipgloss.JoinVertical(lipgloss.Left, d.subTabs(), "", d.vp.View())
}

func (d detailModel) subTabs() string {
	on := lipgloss.NewStyle().Bold(true).Foreground(ember).Underline(true)
	off := lipgloss.NewStyle().Foreground(fgDim)
	sep := lipgloss.NewStyle().Foreground(borderDim).Render(" · ")
	var parts []string
	for s := subOverview; s < subCount; s++ {
		if s == d.sub {
			parts = append(parts, on.Render(s.label()))
		} else {
			parts = append(parts, off.Render(s.label()))
		}
	}
	return strings.Join(parts, sep)
}

func (d detailModel) overview(sc *model.Scenario) string {
	if sc == nil {
		return ""
	}
	label := lipgloss.NewStyle().Foreground(fgDim)
	val := lipgloss.NewStyle().Foreground(fg)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff"))
	field := func(name, v string, valStyle lipgloss.Style) string {
		if v == "" {
			v = "—"
		}
		return label.Render(name) + "\n" + valStyle.Render(v)
	}
	recs, _ := history.Load(history.Path(d.set.Root))
	file := sc.File
	if rel := strings.TrimPrefix(file, d.set.Root+"/"); rel != "" {
		file = rel
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		field("Name", sc.Name, title),
		"",
		label.Render("Last run")+"\n"+lastRunLine(recs, sc.Name, time.Now()),
		"",
		field("Description", sc.Description, val),
		"",
		field("Tags", strings.Join(sc.Tags, ", "), val),
		"",
		field("Suite", sc.Suite, val),
		"",
		field("Shape", scenarioShape(sc), val),
		"",
		field("File", file, val),
	)
}
