// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/shinari-dev/shinari/cli/history"
	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/engine"
	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/core/validate"
)

// reDiscoverMsg triggers a project reload + re-validation (after an edit/scaffold).
type reDiscoverMsg struct{}

// editDoneMsg signals the external editor exited.
type editDoneMsg struct{ err error }

// program is the active app program; the run goroutine streams EventMsgs via
// program.Send. One app runs at a time. Set by RunApp.
var program *tea.Program

type tab int

const (
	tabProject tab = iota
	tabScenarios
)

// App is the root model of the interactive control center.
type App struct {
	set    *discover.Set
	tab    tab
	list   list.Model
	detail detailModel
	keys   keyMap
	width  int
	height int

	run         *runModel
	RunFn       func(ctx context.Context, sc *model.Scenario, send func(engine.Event)) (engine.RunResult, error)
	DryStreamFn func(ctx context.Context, sc *model.Scenario, send func(engine.Event)) (engine.RunResult, error)
	RunSetFn    func(ctx context.Context, targets []string, send func(engine.Event)) (engine.RunResult, error)
	After       func(engine.RunResult, []engine.Event) error
	History     []history.Record

	// Editor is the user's $EDITOR, injected by the CLI (cli/tui never reads env).
	Editor string
	// Version is the shinari build version, shown in the top bar (injected by the CLI).
	Version         string
	validateSummary string
	newForm         *newModel
	showSplash      bool
	scen            twoPane         // table (top) + detail (bottom)
	confirmDelete   *model.Scenario // pending delete awaiting y/n
}

// scenSplit is the share of the Scenarios body height given to the table pane.
const scenSplit = 45

// NewApp builds the root model from a discovered project.
func NewApp(set *discover.Set) App {
	items := make([]list.Item, 0, len(set.Scenarios))
	for _, sc := range set.Scenarios {
		items = append(items, scenarioItem{sc: sc})
	}
	l := list.New(items, scenarioDelegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)      // the app renders its own footer
	l.SetShowStatusBar(false) // no "N items" line; the table header covers it
	return App{
		set:        set,
		tab:        tabScenarios,
		list:       l,
		detail:     newDetail(set),
		keys:       defaultKeys(),
		showSplash: true,
		scen:       newTwoPane(paneSpec{"Scenarios", steel}, paneSpec{"Overview", ember}, scenSplit),
	}
}

// sizeScenarios fits the table and detail panes to the current twoPane geometry.
func (a *App) sizeScenarios() {
	topH, bottomH := a.scen.paneHeights()
	a.list.SetDelegate(scenarioDelegate{recs: a.History, inner: a.width - 2})
	listH := paneContentHeight(topH) - 1 // header row
	if listH < 0 {
		listH = 0
	}
	a.list.SetSize(a.width-2, listH)
	a.detail.setSize(a.width, bottomH)
}

// SetExplainFn injects the explain renderer (keeps cli/tui out of package main).
func (a *App) SetExplainFn(fn func(*model.Scenario) string) { a.detail.ExplainFn = fn }

func (a App) Tab() string {
	if a.tab == tabProject {
		return "project"
	}
	return "scenarios"
}

func (a App) Selected() *model.Scenario {
	if it, ok := a.list.SelectedItem().(scenarioItem); ok {
		return it.sc
	}
	return nil
}

func (a App) Init() tea.Cmd { return nil }

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		// Size the body to what's left after status + tab strip (1) + key bar (1)
		// + slack (1).
		statusH := lipgloss.Height(renderStatusBar(a.statusLabel(), msg.Width))
		bodyH := msg.Height - statusH - 3
		// Scenarios screen: table pane over detail pane via the shared twoPane.
		a.scen.setSize(msg.Width, bodyH)
		a.sizeScenarios()
		a.detail.scenario = a.Selected()
		a.detail = a.detail.gotoSub(a.detail.sub) // load content into the viewport
		if a.run != nil {
			a.run.setSize(msg.Width, bodyH+1) // run view omits the tab strip → +1 row
		}
		return a, nil
	case EventMsg, DoneMsg:
		if a.run != nil {
			nr, cmd := a.run.Update(msg)
			a.run = &nr
			return a, cmd
		}
		return a, nil
	case reDiscoverMsg:
		if ns, err := discover.Load(a.set.Root); err == nil {
			a.set = ns
			items := make([]list.Item, 0, len(ns.Scenarios))
			for _, sc := range ns.Scenarios {
				items = append(items, scenarioItem{sc: sc})
			}
			a.list.SetItems(items)
			a.validateSummary = summarizeValidate(validate.Validate(ns))
			// Re-bind the detail to the reloaded set + (possibly changed)
			// selection and reload its content, so it never shows stale or empty
			// data after an edit/create/delete.
			a.detail.set = ns
			a.detail.scenario = a.Selected()
			a.detail = a.detail.gotoSub(a.detail.sub)
		}
		return a, nil
	case editDoneMsg:
		if msg.err != nil {
			a.validateSummary = lipgloss.NewStyle().Foreground(fail).Render(msg.err.Error())
			return a, nil
		}
		return a, func() tea.Msg { return reDiscoverMsg{} }
	case createdMsg:
		if msg.err != nil {
			a.validateSummary = lipgloss.NewStyle().Foreground(fail).Render(msg.err.Error())
			return a, nil
		}
		if a.Editor == "" {
			return a, func() tea.Msg { return reDiscoverMsg{} }
		}
		c := exec.Command(a.Editor, msg.path) //nolint:gosec // the user's own editor on their own file
		return a, tea.ExecProcess(c, func(err error) tea.Msg { return editDoneMsg{err: err} })
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return a, tea.Quit
		}
		if a.showSplash {
			a.showSplash = false
			return a, nil
		}
		if a.newForm != nil {
			switch {
			case key.Matches(msg, a.keys.Back):
				a.newForm = nil
				return a, nil
			case msg.String() == "enter":
				cmd := a.newForm.submit(a.set.Root)
				a.newForm = nil
				return a, cmd
			}
			nf, cmd := a.newForm.Update(msg)
			a.newForm = &nf
			return a, cmd
		}
		// While the scenario search/filter is open, every key feeds the filter box
		// (so letters like q/n/r are typed, not treated as shortcuts).
		if a.tab == tabScenarios && a.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			a.list, cmd = a.list.Update(msg)
			a.detail.scenario = a.Selected()
			return a, cmd
		}
		// A pending delete captures the next key: y deletes, n/esc cancels.
		if a.confirmDelete != nil {
			switch msg.String() {
			case "y", "Y":
				sc := a.confirmDelete
				a.confirmDelete = nil
				if sc.File != "" {
					_ = os.Remove(sc.File)
				}
				return a, func() tea.Msg { return reDiscoverMsg{} }
			default: // n, esc, anything else cancels
				a.confirmDelete = nil
				return a, nil
			}
		}
		if key.Matches(msg, a.keys.Quit) {
			return a, tea.Quit
		}
		if a.run != nil {
			switch {
			case key.Matches(msg, a.keys.Cancel):
				if a.run.cancel != nil {
					a.run.cancel()
				}
				return a, nil
			case key.Matches(msg, a.keys.Back) && a.run.done:
				a.run = nil
				return a, nil
			}
			nr, cmd := a.run.Update(msg)
			a.run = &nr
			return a, cmd
		}
		if key.Matches(msg, a.keys.EditPrj) {
			return a.editProject()
		}
		if a.tab == tabScenarios && key.Matches(msg, a.keys.New) {
			nf := newNewModel()
			a.newForm = &nf
			return a, nil
		}
		if a.tab == tabScenarios && key.Matches(msg, a.keys.Delete) {
			if sc := a.Selected(); sc != nil {
				a.confirmDelete = sc
			}
			return a, nil
		}
		if a.tab == tabScenarios && key.Matches(msg, a.keys.Edit) {
			return a.editSelected()
		}
		if a.tab == tabProject && key.Matches(msg, a.keys.Edit) {
			return a.editProject()
		}
		if a.tab == tabScenarios && key.Matches(msg, a.keys.Run) {
			return a.startRun()
		}
		if a.tab == tabScenarios && key.Matches(msg, a.keys.RunAll) {
			return a.startRunAll()
		}
		if a.tab == tabScenarios && key.Matches(msg, a.keys.DryRun) {
			return a.startDryRun()
		}
		if key.Matches(msg, a.keys.ShiftTab) {
			a.tab = (a.tab + 1) % 2
			return a, nil
		}
		if a.tab == tabScenarios {
			if key.Matches(msg, a.keys.Tab) {
				a.scen.toggleFocus()
				a.sizeScenarios()
				return a, nil
			}
			if key.Matches(msg, a.keys.Fullscreen) {
				a.scen.toggleFull()
				a.sizeScenarios()
				return a, nil
			}
			switch msg.String() {
			case "left", "right":
				a.detail.scenario = a.Selected()
				var cmd tea.Cmd
				a.detail, cmd = a.detail.Update(msg) // switch sub-tab from either pane
				return a, cmd
			case "/":
				var cmd tea.Cmd
				a.list, cmd = a.list.Update(msg)
				a.detail.scenario = a.Selected()
				return a, cmd
			case "up", "down", "k", "j":
				if a.scen.bottomFocused() { // detail focused → scroll its content
					var cmd tea.Cmd
					a.detail, cmd = a.detail.Update(msg)
					return a, cmd
				}
				var cmd tea.Cmd // table focused → move selection, detail follows
				a.list, cmd = a.list.Update(msg)
				a.detail.scenario = a.Selected()
				a.detail = a.detail.gotoSub(a.detail.sub)
				return a, cmd
			default:
				var cmd tea.Cmd
				a.detail, cmd = a.detail.Update(msg) // pgup/pgdn scroll the detail
				return a, cmd
			}
		}
		return a, nil
	default:
		// Forward everything else (notably the list's async FilterMatchesMsg and
		// spinner ticks during search) to the list so filtering actually applies.
		var cmd tea.Cmd
		a.list, cmd = a.list.Update(msg)
		return a, cmd
	}
}

func (a App) View() string {
	if a.showSplash {
		return renderLogo(a.width)
	}
	active := "Scenarios"
	if a.tab == tabProject {
		active = "Project"
	}
	var prj *model.Project
	if a.set != nil && a.set.Project != nil {
		prj = a.set.Project
	}
	status := renderStatusBar(a.statusLabel(), a.width)
	tabs := renderTabs(active)
	bodyH := a.height - lipgloss.Height(status) - 3
	var body, footer string
	switch {
	case a.newForm != nil:
		body = a.newForm.View()
		footer = keyHint([2]string{"⇥", "field"}, [2]string{"^t", "template"}, [2]string{"↵", "create"}, [2]string{"esc", "cancel"})
	case a.run != nil:
		body = a.run.View() // runModel renders its own stacked, bordered panes
		if a.run.done {
			footer = keyHint([2]string{"⇥", "focus"}, [2]string{"↑↓", "scroll"}, [2]string{"f", "full"}, [2]string{"esc", "back"}, [2]string{"q", "quit"})
		} else {
			footer = keyHint([2]string{"⇥", "focus"}, [2]string{"↑↓", "scroll"}, [2]string{"f", "full"}, [2]string{"x", "cancel"}, [2]string{"q", "quit"})
		}
	case a.tab == tabProject:
		body = panelStyle(a.width, bodyH, true).MaxHeight(bodyH).Render(projectContent(prj))
		footer = keyHint([2]string{"⇧e", "edit"}, [2]string{"⇧⇥", "screen"}, [2]string{"q", "quit"})
	case a.tab == tabScenarios:
		a.scen.bottom.title = a.detail.sub.label() // bottom title tracks the sub-tab
		topContent := lipgloss.JoinVertical(lipgloss.Left, scenarioHeader(a.width-2), a.list.View())
		body = a.scen.render(topContent, a.detail.inner(a.Selected()))
		if a.validateSummary != "" {
			body = lipgloss.JoinVertical(lipgloss.Left, body, a.validateSummary)
		}
		footer = keyHint([2]string{"⇥", "focus"}, [2]string{"f", "full"}, [2]string{"↑↓", "move"}, [2]string{"←→", "section"},
			[2]string{"/", "search"}, [2]string{"r", "run"}, [2]string{"⇧r", "all"}, [2]string{"d", "dry"},
			[2]string{"n", "new"}, [2]string{"⇧D", "del"}, [2]string{"⇧⇥", "screen"}, [2]string{"q", "quit"})
		if a.confirmDelete != nil {
			footer = lipgloss.NewStyle().Foreground(fail).Bold(true).Render("delete "+a.confirmDelete.Name+"?") +
				lipgloss.NewStyle().Foreground(fgDim).Render("  (y/n)")
		}
	}
	// The run view drops the top-level tab strip — those screens are unreachable
	// mid-run, so it's dead chrome there.
	if a.run != nil {
		return lipgloss.JoinVertical(lipgloss.Left, status, body, footer)
	}
	return lipgloss.JoinVertical(lipgloss.Left, status, tabs, body, footer)
}

// projectContent renders the project overview: name, description, configured
// providers, and the source file.
func projectContent(p *model.Project) string {
	if p == nil {
		return lipgloss.NewStyle().Foreground(fgDim).Render("no project loaded")
	}
	label := lipgloss.NewStyle().Foreground(fgDim)
	val := lipgloss.NewStyle().Foreground(fg)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff"))
	dash := func(s string) string {
		if s == "" {
			return "—"
		}
		return s
	}
	provs := make([]string, 0, len(p.Providers))
	for k := range p.Providers {
		provs = append(provs, k)
	}
	sort.Strings(provs)
	return lipgloss.JoinVertical(lipgloss.Left,
		label.Render("Name"), title.Render(dash(p.Name)), "",
		label.Render("Description"), val.Render(dash(p.Description)), "",
		label.Render("Providers"), val.Render(dash(strings.Join(provs, ", "))), "",
		label.Render("Source"), val.Render(dash(p.File)),
	)
}

// statusLabel is the top-bar label: the tool name and build version.
func (a App) statusLabel() string {
	if a.Version == "" {
		return "shinari"
	}
	return "shinari - " + a.Version
}

// launchEditor opens path in the user's editor. It honors $EDITOR (possibly
// with flags, e.g. "code -w") and falls back to vi so editing works even when
// no editor is configured. Returns nil if there is nothing to edit.
func (a App) launchEditor(path string) tea.Cmd {
	if path == "" {
		return nil
	}
	fields := strings.Fields(a.Editor)
	if len(fields) == 0 {
		fields = []string{"vi"}
	}
	args := append(fields[1:], path)
	c := exec.Command(fields[0], args...) //nolint:gosec // the user's own editor on their own file
	return tea.ExecProcess(c, func(err error) tea.Msg { return editDoneMsg{err: err} })
}

// editSelected edits the selected scenario's file.
func (a App) editSelected() (tea.Model, tea.Cmd) {
	if sc := a.Selected(); sc != nil {
		return a, a.launchEditor(sc.File)
	}
	return a, nil
}

// editProject edits the project file (project.yml).
func (a App) editProject() (tea.Model, tea.Cmd) {
	if a.set != nil && a.set.Project != nil {
		return a, a.launchEditor(a.set.Project.File)
	}
	return a, nil
}

// summarizeValidate renders a one-line error/warning tally for the inline panel.
func summarizeValidate(fs []validate.Finding) string {
	errs, warns := 0, 0
	for _, f := range fs {
		if f.Severity == validate.Error {
			errs++
		} else {
			warns++
		}
	}
	ok := lipgloss.NewStyle().Foreground(pass).Render("✓")
	w := lipgloss.NewStyle().Foreground(warn).Render("⚠")
	return fmt.Sprintf("%s %d errors   %s %d warnings", ok, errs, w, warns)
}

// runArea is the full area the run view paints into (it draws its own pane
// borders). Used to size the run view when a run starts; a WindowSizeMsg later
// keeps it in sync on resize.
func (a App) runArea() (int, int) {
	statusH := lipgloss.Height(renderStatusBar(a.statusLabel(), a.width))
	// The run view hides the tab strip (it's unreachable mid-run), so it gets
	// one more row than the tabbed screens.
	bodyH := a.height - statusH - 2
	return a.width, bodyH
}

func (a App) startRun() (tea.Model, tea.Cmd) {
	return a.launchRun(a.RunFn, a.After, false)
}

// startDryRun streams a dry-run (actions skipped) through the same live run view;
// it is not recorded to history.
func (a App) startDryRun() (tea.Model, tea.Cmd) {
	return a.launchRun(a.DryStreamFn, nil, true)
}

// startRunAll runs every scenario currently visible in the list (i.e. the
// search-filtered set, or the whole suite when no filter is active) as one run.
func (a App) startRunAll() (tea.Model, tea.Cmd) {
	if a.RunSetFn == nil {
		return a, nil
	}
	var targets []string
	for _, it := range a.list.VisibleItems() {
		if si, ok := it.(scenarioItem); ok {
			targets = append(targets, si.sc.Name)
		}
	}
	if len(targets) == 0 {
		return a, nil
	}
	rm := newRun()
	rm.AfterRun = a.After
	ctx, cancel := context.WithCancel(context.Background())
	rm.cancel = cancel
	w, h := a.runArea()
	rm.setSize(w, h)
	a.run = &rm
	fn := a.RunSetFn
	cmd := func() tea.Msg {
		res, err := fn(ctx, targets, func(e engine.Event) { program.Send(EventMsg{Event: e}) })
		return DoneMsg{Res: res, Err: err}
	}
	return a, cmd
}

// launchRun starts a streaming run sub-model driven by fn; after (may be nil)
// runs on completion. dry labels the view as a dry-run.
func (a App) launchRun(fn func(context.Context, *model.Scenario, func(engine.Event)) (engine.RunResult, error), after func(engine.RunResult, []engine.Event) error, dry bool) (tea.Model, tea.Cmd) {
	sc := a.Selected()
	if sc == nil || fn == nil {
		return a, nil
	}
	rm := newRun()
	rm.AfterRun = after
	rm.dry = dry
	ctx, cancel := context.WithCancel(context.Background())
	rm.cancel = cancel
	w, h := a.runArea()
	rm.setSize(w, h)
	a.run = &rm
	cmd := func() tea.Msg {
		res, err := fn(ctx, sc, func(e engine.Event) { program.Send(EventMsg{Event: e}) })
		return DoneMsg{Res: res, Err: err}
	}
	return a, cmd
}
