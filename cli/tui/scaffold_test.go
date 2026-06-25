// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/core/validate"

	// register the built-in providers so validate resolves exec/etc., exactly
	// as the real binary does via cli/wiring.go (test-only dependency).
	_ "github.com/shinari-dev/shinari/providers/all"
)

// writeProject drops a minimal kind:Project root so discover.Load succeeds.
func writeProject(t *testing.T, root string) {
	t.Helper()
	yml := "apiVersion: shinari/v1\nkind: Project\nname: test\nproviders:\n  exec: {}\n"
	if err := os.WriteFile(filepath.Join(root, "project.yml"), []byte(yml), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}
}

func TestScenarioTemplateParsesAndValidates(t *testing.T) {
	for _, kind := range []string{"minimal", "fault-inject"} {
		yml := scenarioTemplate("cache-outage", kind)
		sc, err := model.ParseScenario([]byte(yml), "cache-outage.yml")
		if err != nil {
			t.Fatalf("%s template did not parse: %v", kind, err)
		}
		if sc.Name == "" {
			t.Fatalf("%s template missing name", kind)
		}
	}
}

func TestWriteScenarioCreatesValidProject(t *testing.T) {
	root := t.TempDir()
	writeProject(t, root)
	path, err := writeScenario(root, "resilience", "cache-outage", "minimal")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if filepath.Base(path) != "cache-outage.yml" {
		t.Fatalf("unexpected path %s", path)
	}
	set, err := discover.Load(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	for _, f := range validate.Validate(set) {
		if f.Severity == validate.Error {
			t.Fatalf("scaffolded scenario has a validate error: %s", f.String())
		}
	}
	if _, err := writeScenario(root, "resilience", "cache-outage", "minimal"); err == nil {
		t.Fatal("writing over an existing file should error")
	}
}
