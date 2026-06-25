// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/shinari-dev/shinari/cli/golden"
	"github.com/shinari-dev/shinari/cli/history"
	"github.com/shinari-dev/shinari/cli/render"
	"github.com/shinari-dev/shinari/core/engine"
)

func newRunCmd(project, color *string, stdout, stderr io.Writer, getenv func(string) string, lookupEnv func(string) (string, bool)) *cobra.Command {
	var out, include, exclude string
	var dryRun, keepUp, verbose, update, record bool
	cmd := &cobra.Command{
		Use:   "run [target...]",
		Short: "execute scenarios (target = scenario name or suite)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := cmdRun(*project, *color, out, args, dryRun, keepUp, verbose, update, record, include, exclude, stdout, stderr, getenv, lookupEnv); code != 0 {
				return &exitError{code}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "shinari-out", "report output directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "skip actions, run probes/assertions only")
	cmd.Flags().BoolVar(&keepUp, "keep-up", false, "skip teardown, preserving the stack for inspection")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "stream per-step values, durations, and section banners")
	cmd.Flags().StringVar(&include, "include-tags", "", "run only scenarios matching this tag expression")
	cmd.Flags().StringVar(&exclude, "exclude-tags", "", "exclude scenarios matching this tag expression")
	cmd.Flags().BoolVarP(&update, "update", "u", false, "write/refresh the findings ledger (shinari.findings.yml) from this run")
	cmd.Flags().BoolVar(&record, "record", false, "append a run-record to shinari-history.jsonl for `shinari log`")
	return cmd
}

func cmdRun(dir, color, out string, targets []string, dryRun, keepUp, verbose, update, record bool, include, exclude string, stdout, stderr io.Writer, getenv func(string) string, lookupEnv func(string) (string, bool)) int {
	set, ok := load(dir, stderr)
	if !ok {
		return 2 // could not even establish the harness
	}
	pal := paletteFor(color, stdout, lookupEnv)

	resolvedEnv, eerr := resolveEnv(set.Project.Env, lookupEnv)
	if eerr != nil {
		fmt.Fprintln(stderr, eerr)
		return 2 // ERRORED: setup precondition, not a usage error
	}

	unlock, err := lockRun(set.Root)
	if err != nil {
		fmt.Fprintf(stderr, "another shinari run is active for this project: %v\n", err)
		return 2
	}
	defer unlock()

	rec := &engine.Recorder{}
	opts := engine.Options{
		KeepUp:      keepUp || getenv("KEEP_UP") == "1",
		DryRun:      dryRun,
		IncludeTags: include,
		ExcludeTags: exclude,
		Env:         resolvedEnv,
	}
	console := &render.Console{W: stdout, Verbose: verbose, Pal: pal}
	res, err := engine.Run(context.Background(), set, targets, engine.Multi(rec, console), opts)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	if len(res.Scenarios) == 0 {
		fmt.Fprintln(stdout, "no scenarios matched")
		return 0
	}
	render.Summary(stdout, res, pal)

	if werr := writeReports(out, res, rec.Events); werr != nil {
		fmt.Fprintln(stderr, werr)
		return 2
	}
	fmt.Fprintln(stdout, pal.Dim(fmt.Sprintf("reports → %s/{results.tsv,results.json,junit.xml,journal.jsonl,findings.md}", out)))

	exit := res.Verdict().ExitCode()

	var observed []engine.FindingRecord
	for _, sc := range res.Scenarios {
		observed = append(observed, sc.Findings...)
	}
	if record {
		hrec := history.Record{
			RunID:    res.Start.UTC().Format(time.RFC3339Nano),
			Time:     res.Start,
			Verdict:  string(res.Verdict()),
			Duration: res.End.Sub(res.Start),
		}
		for _, f := range observed {
			hrec.Findings = append(hrec.Findings, history.Finding{
				ID: f.ID, Scenario: f.Scenario, Narrative: f.Narrative, NowPasses: f.NowPasses,
			})
		}
		historyPath := history.Path(set.Root)
		if herr := history.Append(historyPath, hrec); herr != nil {
			fmt.Fprintln(stderr, herr)
			return 2
		}
		fmt.Fprintln(stdout, pal.Dim("run recorded → "+historyPath))
	}

	goldenPath := filepath.Join(set.Root, "shinari.findings.yml")

	if update {
		if werr := golden.Write(goldenPath, golden.FromObserved(observed)); werr != nil {
			fmt.Fprintln(stderr, werr)
			return 2
		}
		fmt.Fprintln(stdout, pal.Dim("findings ledger updated → "+goldenPath))
		return exit
	}

	g, exists, gerr := golden.Load(goldenPath)
	if gerr != nil {
		fmt.Fprintln(stderr, gerr)
		return 2
	}
	if exists {
		if drift := golden.Reconcile(g, observed); !drift.Empty() {
			fmt.Fprintln(stdout, pal.Bold("findings ledger drift:"))
			_ = drift.Report(stdout)
			if exit == 0 {
				exit = 1
			}
		}
	}
	return exit
}

func writeReports(out string, res engine.RunResult, events []engine.Event) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	files := map[string]func(io.Writer) error{
		"results.tsv":    func(w io.Writer) error { return render.TSV(w, res) },
		"results.json":   func(w io.Writer) error { return render.ResultsJSON(w, res) },
		"junit.xml":      func(w io.Writer) error { return render.JUnit(w, res) },
		"journal.jsonl":  func(w io.Writer) error { return render.Journal(w, events) },
		"findings.md":    func(w io.Writer) error { return render.FindingsReport(w, res) },
	}
	for name, fn := range files {
		f, err := os.Create(filepath.Join(out, name))
		if err != nil {
			return err
		}
		if err := fn(f); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

// lockRun is the flock single-run guard, keyed by project path.
func lockRun(projectDir string) (func(), error) {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("shinari-%x.lock", sha256.Sum256([]byte(abs)))
	path := filepath.Join(os.TempDir(), key)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock %s held", path)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
