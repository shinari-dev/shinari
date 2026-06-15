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
  - { run: assert, with: { of: "${.total}", equals: 1 }, desc: "exactly once" }
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

func TestUsageErrorIs64(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut, noEnv); code != 64 {
		t.Fatalf("no command: code = %d", code)
	}
	if code := run([]string{"frobnicate"}, &out, &errOut, noEnv); code != 64 {
		t.Fatalf("unknown command: code = %d", code)
	}
}

func TestListGroupsBySuite(t *testing.T) {
	dir := project(t, false)
	var out, errOut bytes.Buffer
	if code := run([]string{"-C", dir, "list"}, &out, &errOut, noEnv); code != 0 {
		t.Fatalf("code = %d, err: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "core") || !strings.Contains(out.String(), "exactly-once") {
		t.Errorf("list output: %s", out.String())
	}
}

func TestValidateCleanProject(t *testing.T) {
	dir := project(t, false)
	var out, errOut bytes.Buffer
	if code := run([]string{"-C", dir, "validate"}, &out, &errOut, noEnv); code != 0 {
		t.Fatalf("code = %d: %s%s", code, out.String(), errOut.String())
	}
}

func TestValidateBrokenProjectExits1(t *testing.T) {
	dir := project(t, false)
	_ = os.WriteFile(filepath.Join(dir, "bad.yml"),
		[]byte("apiVersion: shinari/v1\nkind: Scenario\nname: bad\nverify:\n  - { run: ghost.poke }\n"), 0o644)
	var out, errOut bytes.Buffer
	if code := run([]string{"-C", dir, "validate"}, &out, &errOut, noEnv); code != 1 {
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
	code := run([]string{"-C", dir, "-out", outDir, "run"}, &out, &errOut, noEnv)
	if code != 0 {
		t.Fatalf("code = %d: %s%s", code, out.String(), errOut.String())
	}
	for _, f := range []string{"results.tsv", "results.json", "junit.xml", "journal.jsonl", "findings.md"} {
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
	code := run([]string{"-C", dir, "-out", outDir, "run"}, &out, &errOut, noEnv)
	if code != 1 {
		t.Fatalf("code = %d: %s%s", code, out.String(), errOut.String())
	}
}

func TestRunUnknownTargetIsUsageError(t *testing.T) {
	dir := project(t, false)
	var out, errOut bytes.Buffer
	code := run([]string{"-C", dir, "-out", t.TempDir(), "run", "zzz"}, &out, &errOut, noEnv)
	if code != 64 {
		t.Fatalf("code = %d", code)
	}
}

func TestInitWritesLockFile(t *testing.T) {
	dir := project(t, false)
	var out, errOut bytes.Buffer
	if code := run([]string{"-C", dir, "init"}, &out, &errOut, noEnv); code != 0 {
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
