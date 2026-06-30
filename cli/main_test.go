// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shinari-dev/shinari/sdk"
)

// okProvider is a fake lifecycle SUT for CLI-level tests.
type okProvider struct{ fail bool }

func (p okProvider) Type() string                   { return "clifake" }
func (p okProvider) Configure(map[string]any) error { return nil }
func (p okProvider) Verbs() []sdk.VerbSpec {
	return []sdk.VerbSpec{
		{Name: "up", Kind: sdk.KindAction, SideEffects: true, Primary: "services"},
		{Name: "down", Kind: sdk.KindAction, SideEffects: true},
		{Name: "count", Kind: sdk.KindProbe, Primary: "job"},
	}
}
func (p okProvider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	if verb == "count" {
		if p.fail {
			return sdk.VerbResult{Value: 2}, nil
		}
		return sdk.VerbResult{Value: 1}, nil
	}
	return sdk.VerbResult{Value: "ok"}, nil
}

// cliFail sets whether the registered "clifake" reports a duplicate (count
// 2). Registration is once; project() sets this before the run builds the
// registry. (sdk.Register panics on a duplicate.)
var cliFail bool

func init() { sdk.Register("clifake", func() sdk.Provider { return okProvider{fail: cliFail} }) }

func project(t *testing.T, countIsTwo bool) string {
	t.Helper()
	cliFail = countIsTwo
	dir := t.TempDir()
	files := map[string]string{
		"project.yml": "apiVersion: shinari/v1\nkind: Project\nname: demo\nproviders:\n  sut: { source: clifake }\n",
		"scenarios/core/once.yml": `apiVersion: shinari/v1
kind: Scenario
name: exactly-once
description: the job must run exactly once
setup:
  - { run: sut.up, with: [app] }
verify:
  - { run: sut.count, with: job, as: total }
  - { run: assert, with: { of: "${.outputs.total.value}", equals: 1 }, desc: "exactly once" }
`,
		"providers/app.yml": `apiVersion: shinari/v1
kind: Provider
name: app
verbs:
  total: { probe: { run: sut.count, with: job } }
`,
	}
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func noEnv(string) string { return "" }

func noLookup(string) (string, bool) { return "", false }

func TestUsageErrorIs64(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut, noEnv, noLookup); code != 64 {
		t.Fatalf("no command: code = %d", code)
	}
	if code := run([]string{"frobnicate"}, &out, &errOut, noEnv, noLookup); code != 64 {
		t.Fatalf("unknown command: code = %d", code)
	}
}

func TestListGroupsBySuite(t *testing.T) {
	dir := project(t, false)
	var out, errOut bytes.Buffer
	if code := run([]string{"--project", dir, "list"}, &out, &errOut, noEnv, noLookup); code != 0 {
		t.Fatalf("code = %d, err: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "core") || !strings.Contains(out.String(), "exactly-once") {
		t.Errorf("list output: %s", out.String())
	}
}

func TestValidateCleanProject(t *testing.T) {
	dir := project(t, false)
	var out, errOut bytes.Buffer
	if code := run([]string{"--project", dir, "validate"}, &out, &errOut, noEnv, noLookup); code != 0 {
		t.Fatalf("code = %d: %s%s", code, out.String(), errOut.String())
	}
}

func TestValidateBrokenProjectExits1(t *testing.T) {
	dir := project(t, false)
	_ = os.WriteFile(filepath.Join(dir, "bad.yml"),
		[]byte("apiVersion: shinari/v1\nkind: Scenario\nname: bad\nverify:\n  - { run: ghost.poke }\n"), 0o644)
	var out, errOut bytes.Buffer
	if code := run([]string{"--project", dir, "validate"}, &out, &errOut, noEnv, noLookup); code != 1 {
		t.Fatalf("code = %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "rule 3") {
		t.Errorf("output: %s", out.String())
	}
}

func TestRunPassedWritesReportsAndExits0(t *testing.T) {
	dir := project(t, false)
	outDir := filepath.Join(t.TempDir(), "reports")
	var out, errOut bytes.Buffer
	code := run([]string{"--project", dir, "--out", outDir, "run"}, &out, &errOut, noEnv, noLookup)
	if code != 0 {
		t.Fatalf("code = %d: %s%s", code, out.String(), errOut.String())
	}
	for _, f := range []string{"results.tsv", "results.json", "junit.xml", "journal.jsonl", "findings.md", "findings.sarif"} {
		if _, err := os.Stat(filepath.Join(outDir, f)); err != nil {
			t.Errorf("missing report %s", f)
		}
	}
	junit, _ := os.ReadFile(filepath.Join(outDir, "junit.xml"))
	if !strings.Contains(string(junit), `classname="exactly-once"`) {
		t.Errorf("junit.xml: %s", junit)
	}
}

func TestRunFailedExits1(t *testing.T) {
	dir := project(t, true) // count returns 2 → regression
	outDir := filepath.Join(t.TempDir(), "reports")
	var out, errOut bytes.Buffer
	code := run([]string{"--project", dir, "--out", outDir, "run"}, &out, &errOut, noEnv, noLookup)
	if code != 1 {
		t.Fatalf("code = %d: %s%s", code, out.String(), errOut.String())
	}
}

func TestRunRequiredEnvUnsetExits2(t *testing.T) {
	dir := project(t, false)
	// Declare a required env var (null default) the project does not set.
	_ = os.WriteFile(filepath.Join(dir, "project.yml"),
		[]byte("apiVersion: shinari/v1\nkind: Project\nname: demo\nproviders:\n  sut: { source: clifake }\nenv:\n  DATABASE_URL:\n"), 0o644)
	var out, errOut bytes.Buffer
	code := run([]string{"--project", dir, "--out", t.TempDir(), "run"}, &out, &errOut, noEnv, noLookup)
	if code != 2 {
		t.Fatalf("code = %d, want 2 (ERRORED); err: %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "DATABASE_URL") {
		t.Errorf("error should name the unset var: %s", errOut.String())
	}
}

// INCONCLUSIVE (a steady-state gate that never holds before method) is the one
// verdict→exit branch the other run tests (0/1/2/64) leave unchecked.
func TestRunInconclusiveExits3(t *testing.T) {
	cliFail = false // count returns 1, so the gate's ==99 assert never holds
	dir := t.TempDir()
	files := map[string]string{
		"project.yml": "apiVersion: shinari/v1\nkind: Project\nname: demo\nproviders:\n  sut: { source: clifake }\n",
		"scenarios/core/gate.yml": `apiVersion: shinari/v1
kind: Scenario
name: never-healthy
setup:
  - { run: sut.up, with: [app] }
steadyState:
  - { run: sut.count, with: job, as: total }
  - { run: assert, with: { of: "${.outputs.total.value}", equals: 99 }, desc: "never healthy" }
method:
  - phase: x
    steps:
      - { run: sut.count, with: job }
`,
	}
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--project", dir, "--out", t.TempDir(), "run"}, &out, &errOut, noEnv, noLookup)
	if code != 3 {
		t.Fatalf("code = %d, want 3 (INCONCLUSIVE); out=%s err=%s", code, out.String(), errOut.String())
	}
}

func TestRunUnknownTargetIsUsageError(t *testing.T) {
	dir := project(t, false)
	var out, errOut bytes.Buffer
	code := run([]string{"--project", dir, "--out", t.TempDir(), "run", "zzz"}, &out, &errOut, noEnv, noLookup)
	if code != 64 {
		t.Fatalf("code = %d", code)
	}
}

func TestInitWritesLockFile(t *testing.T) {
	dir := project(t, false)
	var out, errOut bytes.Buffer
	if code := run([]string{"--project", dir, "init"}, &out, &errOut, noEnv, noLookup); code != 0 {
		t.Fatalf("code = %d: %s", code, errOut.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "shinari.lock.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "builtin") {
		t.Errorf("lock: %s", data)
	}
}

func TestCoreNeverImportsCLIOrExits(t *testing.T) {
	// The core boundary rule: core imports no CLI/colour/argv package and
	// never calls os.Exit. Grep-level enforcement.
	root := "../core"
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		src := string(data)
		if strings.Contains(src, "os.Exit") {
			t.Errorf("%s calls os.Exit — rendering/exit codes are front-end only", path)
		}
		for _, banned := range []string{`"flag"`, `"github.com/shinari-dev/shinari/cli`} {
			if strings.Contains(src, banned) {
				t.Errorf("%s imports %s — core must not know its front end", path, banned)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func taggedProject(t *testing.T) string {
	t.Helper()
	cliFail = false
	dir := t.TempDir()
	files := map[string]string{
		"project.yml": "apiVersion: shinari/v1\nkind: Project\nname: demo\nproviders:\n  sut: { source: clifake }\n",
		"scenarios/core/fast.yml": `apiVersion: shinari/v1
kind: Scenario
name: fast-one
tags: [fast]
setup:
  - { run: sut.up, with: [app] }
verify:
  - { run: sut.count, with: job, as: total }
  - { run: assert, with: { of: "${.outputs.total.value}", equals: 1 } }
`,
		"scenarios/core/slow.yml": `apiVersion: shinari/v1
kind: Scenario
name: slow-one
tags: [slow, flaky]
setup:
  - { run: sut.up, with: [app] }
verify:
  - { run: sut.count, with: job, as: total }
  - { run: assert, with: { of: "${.outputs.total.value}", equals: 1 } }
`,
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestListFiltersByTag(t *testing.T) {
	dir := taggedProject(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--project", dir, "--include-tags", "fast", "list"}, &stdout, &stderr, os.Getenv, os.LookupEnv)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "fast-one") || strings.Contains(out, "slow-one") {
		t.Fatalf("list output did not filter by tag:\n%s", out)
	}
}

func TestRunZeroMatchExitsZero(t *testing.T) {
	dir := taggedProject(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--project", dir, "--include-tags", "missing", "run"}, &stdout, &stderr, os.Getenv, os.LookupEnv)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "no scenarios matched") {
		t.Fatalf("expected 'no scenarios matched', got:\n%s", stdout.String())
	}
}

func TestRunBadTagExprIsUsageError(t *testing.T) {
	dir := taggedProject(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--project", dir, "--include-tags", "slow &", "run"}, &stdout, &stderr, os.Getenv, os.LookupEnv)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d (usage)", code, exitUsage)
	}
}

func TestListFilterFlagAfterSubcommand(t *testing.T) {
	dir := taggedProject(t)
	var stdout, stderr bytes.Buffer
	// Flag AFTER the subcommand must work now (the whole point of pflag).
	code := run([]string{"--project", dir, "list", "--include-tags", "fast"}, &stdout, &stderr, noEnv, noLookup)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "fast-one") || strings.Contains(out, "slow-one") {
		t.Fatalf("filter (flag after subcommand) did not apply:\n%s", out)
	}
}

func TestProjectFlagAfterSubcommand(t *testing.T) {
	dir := project(t, false)
	var stdout, stderr bytes.Buffer
	// Persistent --project is valid in any position.
	code := run([]string{"list", "--project", dir}, &stdout, &stderr, noEnv, noLookup)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "exactly-once") {
		t.Fatalf("list did not run with --project after subcommand:\n%s", stdout.String())
	}
}

func TestProjectShorthand(t *testing.T) {
	dir := project(t, false)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-p", dir, "list"}, &stdout, &stderr, noEnv, noLookup); code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
}

func TestVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr, noEnv, noLookup); code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), version) {
		t.Fatalf("--version output %q does not contain %q", stdout.String(), version)
	}
}
