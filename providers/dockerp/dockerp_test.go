// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package dockerp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stub records its argv and prints canned output.
func stubBin(t *testing.T) (bin string, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "fake-docker")
	argsFile = filepath.Join(dir, "args.txt")
	script := "#!/bin/sh\necho \"$@\" >> " + argsFile + "\necho stub-ok\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argsFile
}

func provider(t *testing.T) (*Provider, string) {
	t.Helper()
	bin, argsFile := stubBin(t)
	p := New().(*Provider)
	if err := p.Configure(map[string]any{
		"composeFiles": []any{"assets/stack.yml"},
		"project":      "chaos-run",
		"bin":          bin,
	}); err != nil {
		t.Fatal(err)
	}
	return p, argsFile
}

func recorded(t *testing.T, argsFile string) string {
	t.Helper()
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func TestUpBuildsComposeCommand(t *testing.T) {
	p, argsFile := provider(t)
	res, err := p.Run(context.Background(), "up", map[string]any{"services": []any{"postgres", "app"}})
	if err != nil {
		t.Fatal(err)
	}
	got := recorded(t, argsFile)
	want := "-f assets/stack.yml -p chaos-run up -d --wait postgres app"
	if got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
	if res.Value != "stub-ok" {
		t.Errorf("value = %v", res.Value)
	}
}

func TestKillUsesSIGKILL(t *testing.T) {
	p, argsFile := provider(t)
	if _, err := p.Run(context.Background(), "kill", map[string]any{"service": "worker-a"}); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, argsFile); !strings.Contains(got, "kill -s SIGKILL worker-a") {
		t.Errorf("argv = %q", got)
	}
}

func TestDownRemovesVolumesAndOrphans(t *testing.T) {
	p, argsFile := provider(t)
	if _, err := p.Run(context.Background(), "down", nil); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, argsFile); !strings.Contains(got, "down -v --remove-orphans") {
		t.Errorf("argv = %q", got)
	}
}

func TestFailureSurfacesOutput(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fail-docker")
	_ = os.WriteFile(bin, []byte("#!/bin/sh\necho 'no such service: ghost' >&2\nexit 1\n"), 0o755)
	p := New().(*Provider)
	_ = p.Configure(map[string]any{"bin": bin})
	_, err := p.Run(context.Background(), "stop", map[string]any{"service": "ghost"})
	if err == nil || !strings.Contains(err.Error(), "no such service") {
		t.Fatalf("want compose stderr in error, got %v", err)
	}
}
