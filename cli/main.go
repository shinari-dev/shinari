// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Command shinari (alias shi) is the v1 front end: it parses argv, drives
// core, renders output, and maps verdicts to exit codes.
package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"

	"github.com/shinari-dev/shinari/cli/render"
	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/engine"
	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/core/registry"
	"github.com/shinari-dev/shinari/core/selector"
	"github.com/shinari-dev/shinari/core/validate"
)

const version = "0.2.0-dev"

// exitUsage is EX_USAGE: distinct from the verdict codes 0..3.
const exitUsage = 64

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	fs := flag.NewFlagSet("shinari", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("C", ".", "project directory")
	out := fs.String("out", "shinari-out", "report output directory (run)")
	dryRun := fs.Bool("dry-run", false, "skip actions, run probes/assertions only")
	includeTags := fs.String("include-tags", "", "run/list only scenarios matching this tag expression")
	excludeTags := fs.String("exclude-tags", "", "exclude scenarios matching this tag expression")
	fs.Usage = func() {
		fmt.Fprintf(stderr, `shinari %s — resilience integration testing

Usage: shinari [flags] <command> [target...]

Commands:
  init        resolve providers, write shinari.lock.yml
  validate    static checks, no run
  list        discovered scenarios, grouped by suite
  run         execute scenarios (target = scenario name or suite)

Flags:
`, version)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return exitUsage
	}
	cmd, targets := rest[0], rest[1:]

	switch cmd {
	case "init":
		return cmdInit(*dir, stdout, stderr)
	case "validate":
		return cmdValidate(*dir, stdout, stderr)
	case "list":
		return cmdList(*dir, *includeTags, *excludeTags, stdout, stderr)
	case "run":
		return cmdRun(*dir, *out, targets, *dryRun, *includeTags, *excludeTags, stdout, stderr, getenv)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", cmd)
		fs.Usage()
		return exitUsage
	}
}

func load(dir string, stderr io.Writer) (*discover.Set, bool) {
	set, err := discover.Load(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return set, false
	}
	return set, true
}

func cmdValidate(dir string, stdout, stderr io.Writer) int {
	set, ok := load(dir, stderr)
	if !ok {
		return 1
	}
	findings := validate.Validate(set)
	errors := 0
	for _, f := range findings {
		fmt.Fprintln(stdout, f.String())
		if f.Severity == validate.Error {
			errors++
		}
	}
	if errors > 0 {
		fmt.Fprintf(stdout, "\n%d error(s), %d warning(s)\n", errors, len(findings)-errors)
		return 1
	}
	fmt.Fprintf(stdout, "valid — %d scenario(s), %d warning(s)\n", len(set.Scenarios), len(findings))
	return 0
}

func cmdList(dir, include, exclude string, stdout, stderr io.Writer) int {
	set, ok := load(dir, stderr)
	if !ok {
		return 1
	}
	scenarios, err := selector.Filter(set.Scenarios, include, exclude)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	bySuite := map[string][]*model.Scenario{}
	for _, sc := range scenarios {
		bySuite[sc.Suite] = append(bySuite[sc.Suite], sc)
	}
	suites := make([]string, 0, len(bySuite))
	for s := range bySuite {
		suites = append(suites, s)
	}
	sort.Strings(suites)
	for _, suite := range suites {
		name := suite
		if name == "" {
			name = "(no suite)"
		}
		fmt.Fprintf(stdout, "%s\n", name)
		scs := bySuite[suite]
		sort.Slice(scs, func(i, j int) bool { return scs[i].Name < scs[j].Name })
		for _, sc := range scs {
			fmt.Fprintf(stdout, "  %s", sc.Name)
			if sc.Description != "" {
				fmt.Fprintf(stdout, " — %s", sc.Description)
			}
			if len(sc.Tags) > 0 {
				fmt.Fprintf(stdout, " [%s]", strings.Join(sc.Tags, ", "))
			}
			fmt.Fprintln(stdout)
		}
	}
	return 0
}

// cmdInit resolves every configured provider and writes the lock file.
// V1 has no registry fetch: built-ins pin the engine version, local
// composed providers pin a content checksum.
func cmdInit(dir string, stdout, stderr io.Writer) int {
	set, ok := load(dir, stderr)
	if !ok {
		return 1
	}
	if _, err := registry.New(set, set.Project.Providers); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	type lockEntry struct {
		Kind     string `yaml:"kind"` // builtin | local
		Version  string `yaml:"version,omitempty"`
		Source   string `yaml:"source,omitempty"`
		Checksum string `yaml:"checksum,omitempty"`
	}
	lock := struct {
		Version   int                  `yaml:"version"`
		Providers map[string]lockEntry `yaml:"providers"`
	}{Version: 1, Providers: map[string]lockEntry{}}

	for name, cfg := range set.Project.Providers {
		if cfg.Use != "" {
			def, err := set.FindLocalProvider(cfg.Use)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			data, err := os.ReadFile(def.File)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			lock.Providers[name] = lockEntry{
				Kind: "local", Source: cfg.Use,
				Checksum: fmt.Sprintf("sha256:%x", sha256.Sum256(data)),
			}
			continue
		}
		lock.Providers[name] = lockEntry{Kind: "builtin", Version: version}
	}
	data, err := yaml.Marshal(lock)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	lockPath := filepath.Join(set.Project.Dir, "shinari.lock.yml")
	if err := os.WriteFile(lockPath, data, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s (%d provider(s))\n", lockPath, len(lock.Providers))
	return 0
}

func cmdRun(dir, out string, targets []string, dryRun bool, include, exclude string, stdout, stderr io.Writer, getenv func(string) string) int {
	set, ok := load(dir, stderr)
	if !ok {
		return 2 // could not even establish the harness
	}

	unlock, err := lockRun(set.Root)
	if err != nil {
		fmt.Fprintf(stderr, "another shinari run is active for this project: %v\n", err)
		return 2
	}
	defer unlock()

	rec := &engine.Recorder{}
	console := &render.Console{W: stdout}
	opts := engine.Options{
		KeepUp:      getenv("KEEP_UP") == "1",
		DryRun:      dryRun,
		IncludeTags: include,
		ExcludeTags: exclude,
	}
	res, err := engine.Run(context.Background(), set, targets, engine.Multi(rec, console), opts)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	if len(res.Scenarios) == 0 {
		fmt.Fprintln(stdout, "no scenarios matched")
		return 0
	}
	render.Summary(stdout, res)

	if werr := writeReports(out, res, rec.Events); werr != nil {
		fmt.Fprintln(stderr, werr)
		return 2
	}
	fmt.Fprintf(stdout, "reports: %s/{results.tsv,results.json,junit.xml,journal.jsonl,findings.md}\n", out)
	return res.Verdict().ExitCode()
}

func writeReports(out string, res engine.RunResult, events []engine.Event) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	files := map[string]func(io.Writer) error{
		"results.tsv":   func(w io.Writer) error { return render.TSV(w, res) },
		"results.json":  func(w io.Writer) error { return render.ResultsJSON(w, res) },
		"junit.xml":     func(w io.Writer) error { return render.JUnit(w, res) },
		"journal.jsonl": func(w io.Writer) error { return render.Journal(w, events) },
		"findings.md":   func(w io.Writer) error { return render.FindingsReport(w, res) },
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
