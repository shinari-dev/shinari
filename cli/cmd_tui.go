// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/shinari-dev/shinari/cli/history"
	"github.com/shinari-dev/shinari/cli/tui"
	"github.com/shinari-dev/shinari/core/engine"
	"github.com/shinari-dev/shinari/core/model"
)

func newTuiCmd(project *string, stdout, stderr io.Writer, getenv func(string) string, lookupEnv func(string) (string, bool)) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "interactive control center: browse, explain, dry-run, run, history",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := cmdTui(*project, stdout, stderr, getenv, lookupEnv); code != 0 {
				return &exitError{code}
			}
			return nil
		},
	}
}

// resolveEditor picks the editor the TUI opens scenarios in: $VISUAL, then
// $EDITOR, then vi so editing works out of the box.
func resolveEditor(getenv func(string) string) string {
	for _, k := range []string{"VISUAL", "EDITOR"} {
		if v := getenv(k); v != "" {
			return v
		}
	}
	return "vi"
}

func cmdTui(dir string, stdout, stderr io.Writer, getenv func(string) string, lookupEnv func(string) (string, bool)) int {
	if !isTerminal(stdout) {
		fmt.Fprintln(stderr, "shinari tui requires a terminal")
		return exitUsage
	}
	set, ok := load(dir, stderr)
	if !ok {
		return 2
	}
	dotenv, derr := dotenvOverlay(set.Root, "")
	if derr != nil {
		fmt.Fprintln(stderr, derr)
		return 2
	}
	resolvedEnv, eerr := resolveEnv(set.Project.Env, layeredLookup(lookupEnv, dotenv))
	if eerr != nil {
		fmt.Fprintln(stderr, eerr)
		return 2
	}
	plan, perr := resolveOutput(set.Project.Output, "", "")
	if perr != nil {
		fmt.Fprintln(stderr, perr)
		return 2
	}

	app := tui.NewApp(set)
	app.Editor = resolveEditor(getenv)
	app.Version = version

	// explain + dry-run are injected so cli/tui stays decoupled from package main.
	app.SetExplainFn(func(sc *model.Scenario) string { return explainString(set, sc) })

	// dry-run streams through the same live run view (actions skipped, not recorded).
	app.DryStreamFn = func(ctx context.Context, sc *model.Scenario, send func(engine.Event)) (engine.RunResult, error) {
		return engine.Run(ctx, set, []string{sc.Name},
			engine.Multi(engine.EmitterFunc(send)), engine.Options{DryRun: true, Env: resolvedEnv})
	}

	// run streams events live and, on completion, writes reports + records history.
	app.RunFn = func(ctx context.Context, sc *model.Scenario, send func(engine.Event)) (engine.RunResult, error) {
		rec := &engine.Recorder{}
		em := engine.Multi(rec, engine.EmitterFunc(send))
		res, err := engine.Run(ctx, set, []string{sc.Name}, em,
			engine.Options{Env: resolvedEnv, KeepUp: getenv("KEEP_UP") == "1"})
		if err == nil {
			_, _ = writeReports(plan, res, rec.Events)
		}
		return res, err
	}

	// run-all runs every target (the visible/filtered set) in one streamed run.
	app.RunSetFn = func(ctx context.Context, targets []string, send func(engine.Event)) (engine.RunResult, error) {
		rec := &engine.Recorder{}
		em := engine.Multi(rec, engine.EmitterFunc(send))
		res, err := engine.Run(ctx, set, targets, em,
			engine.Options{Env: resolvedEnv, KeepUp: getenv("KEEP_UP") == "1"})
		if err == nil {
			_, _ = writeReports(plan, res, rec.Events)
		}
		return res, err
	}
	app.After = func(res engine.RunResult) {
		hrec := history.Record{RunID: res.Start.UTC().String(), Time: res.Start, Verdict: string(res.Verdict()), Duration: res.End.Sub(res.Start)}
		for _, sc := range res.Scenarios {
			hrec.Scenarios = append(hrec.Scenarios, sc.Name)
			for _, f := range sc.Findings {
				hrec.Findings = append(hrec.Findings, history.Finding{
					ID: f.ID, Scenario: f.Scenario, Narrative: f.Narrative, NowPasses: f.NowPasses,
				})
			}
		}
		_ = history.Append(history.Path(set.Root), hrec)
	}

	// load existing history for the History tab.
	if recs, herr := history.Load(history.Path(set.Root)); herr == nil {
		app.History = recs
	}

	if err := tui.RunApp(app); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
