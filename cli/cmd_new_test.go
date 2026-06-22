// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewScaffoldsRunnableProject is the warm-start proof: `new <dir>` emits a
// project that validates and runs green on the first try, with no edits.
func TestNewScaffoldsRunnableProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	var out, errOut bytes.Buffer
	if code := run([]string{"new", dir}, &out, &errOut, noEnv, noLookup); code != 0 {
		t.Fatalf("new: code = %d: %s", code, errOut.String())
	}

	for _, rel := range []string{
		"project.yml", ".gitignore", "README.md",
		"providers/jobstore.yml", "scripts/jobstore.sh",
		"scenarios/core/clean-complete.yml",
		"scenarios/recovery/worker-killed.yml",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("missing scaffolded file %s", rel)
		}
	}

	// The project name is derived from the target directory's basename.
	pj, _ := os.ReadFile(filepath.Join(dir, "project.yml"))
	if !strings.Contains(string(pj), "name: demo") {
		t.Errorf("project.yml name not derived from dir:\n%s", pj)
	}

	// The shell script the providers drive must be executable.
	info, err := os.Stat(filepath.Join(dir, "scripts/jobstore.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("scripts/jobstore.sh is not executable: %v", info.Mode())
	}

	// validate then run the freshly scaffolded project — both green.
	out.Reset()
	errOut.Reset()
	if code := run([]string{"-p", dir, "validate"}, &out, &errOut, noEnv, noLookup); code != 0 {
		t.Fatalf("validate: code = %d: %s%s", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	outDir := filepath.Join(t.TempDir(), "reports")
	if code := run([]string{"-p", dir, "-o", outDir, "run"}, &out, &errOut, noEnv, noLookup); code != 0 {
		t.Fatalf("run: code = %d, want 0 (PASSED): %s%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "finding held") {
		t.Errorf("expected the worker-killed finding to hold:\n%s", out.String())
	}
}

func TestNewRefusesExistingProject(t *testing.T) {
	dir := t.TempDir() // t.TempDir already exists
	if err := os.WriteFile(filepath.Join(dir, "project.yml"), []byte("apiVersion: shinari/v1\nkind: Project\nname: keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"new", dir}, &out, &errOut, noEnv, noLookup); code != exitUsage {
		t.Fatalf("code = %d, want %d (refuse to clobber)", code, exitUsage)
	}
	// The pre-existing file is untouched.
	pj, _ := os.ReadFile(filepath.Join(dir, "project.yml"))
	if !strings.Contains(string(pj), "name: keep") {
		t.Errorf("existing project.yml was overwritten:\n%s", pj)
	}
}

func TestNewRefusesCollidingFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	// A stray README in the otherwise-empty target must block the scaffold
	// rather than be overwritten.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"new", dir}, &out, &errOut, noEnv, noLookup); code != exitUsage {
		t.Fatalf("code = %d, want %d (refuse to clobber)", code, exitUsage)
	}
	if readme, _ := os.ReadFile(filepath.Join(dir, "README.md")); string(readme) != "mine\n" {
		t.Errorf("colliding README was overwritten: %q", readme)
	}
	// Nothing else should have been written either.
	if _, err := os.Stat(filepath.Join(dir, "project.yml")); err == nil {
		t.Errorf("project.yml written despite a collision")
	}
}

func TestNewRequiresDirArg(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"new"}, &out, &errOut, noEnv, noLookup); code != exitUsage {
		t.Fatalf("code = %d, want %d (missing dir arg)", code, exitUsage)
	}
}
