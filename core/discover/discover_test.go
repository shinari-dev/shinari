// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const minimalProject = "apiVersion: shinari/v1\nkind: Project\nname: demo\n"

func TestLoadCollectsByKindAndIgnoresForeignYAML(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "project.yml", minimalProject)
	// filenames are free; nesting is free
	write(t, dir, "scenarios/data-loss/s09.yml",
		"apiVersion: shinari/v1\nkind: Scenario\nname: worker-crash\nverify:\n  - { run: assert, with: { of: 1, equals: 1 } }\n")
	write(t, dir, "deep/nested/anything.yaml",
		"apiVersion: shinari/v1\nkind: Scenario\nname: slow-disk\n")
	write(t, dir, "providers/app.yml",
		"apiVersion: shinari/v1\nkind: Provider\nname: app\nverbs:\n  ping: { probe: { run: http.get, with: { path: / } } }\n")
	write(t, dir, "assets/stack.yml", "services:\n  app:\n    image: nginx\n") // not ours

	set, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if set.Project == nil || set.Project.Name != "demo" {
		t.Fatalf("project: %+v", set.Project)
	}
	if len(set.Scenarios) != 2 {
		t.Fatalf("want 2 scenarios, got %d", len(set.Scenarios))
	}
	if len(set.Providers) != 1 {
		t.Fatalf("want 1 provider, got %d", len(set.Providers))
	}
}

func TestSuiteDerivation(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "project.yml", minimalProject)
	write(t, dir, "scenarios/data-loss/a.yml", "apiVersion: shinari/v1\nkind: Scenario\nname: a\n")
	write(t, dir, "other/b.yml", "apiVersion: shinari/v1\nkind: Scenario\nname: b\n")
	set, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	suites := map[string]string{}
	for _, sc := range set.Scenarios {
		suites[sc.Name] = sc.Suite
	}
	if suites["a"] != "data-loss" {
		t.Errorf("suite(a) = %q, want data-loss", suites["a"])
	}
	if suites["b"] != "other" {
		t.Errorf("suite(b) = %q, want other", suites["b"])
	}
}

func TestMalformedMarkedFileIsErrorNotSkip(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "project.yml", minimalProject)
	// recognized header, malformed body: setup must be a list
	write(t, dir, "bad.yml", "apiVersion: shinari/v1\nkind: Scenario\nname: bad\nsetup: notalist\n")
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("want malformed-resource error, got %v", err)
	}
}

func TestTwoProjectsIsError(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "project.yml", minimalProject)
	write(t, dir, "sub/project2.yml", "apiVersion: shinari/v1\nkind: Project\nname: other\n")
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "Project") {
		t.Fatalf("want two-projects error, got %v", err)
	}
}

func TestNoProjectIsError(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "s.yml", "apiVersion: shinari/v1\nkind: Scenario\nname: s\n")
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "Project") {
		t.Fatalf("want no-project error, got %v", err)
	}
}

func TestFindLocalProvider(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "project.yml", minimalProject)
	write(t, dir, "providers/app.yml",
		"apiVersion: shinari/v1\nkind: Provider\nname: app\nverbs:\n  ping: { probe: { run: http.get, with: { path: / } } }\n")
	set, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	pd, err := set.FindLocalProvider("./providers/app")
	if err != nil || pd.Name != "app" {
		t.Fatalf("pd=%v err=%v", pd, err)
	}
	if _, err := set.FindLocalProvider("./providers/nope"); err == nil {
		t.Fatal("want error for unknown local provider")
	}
}
