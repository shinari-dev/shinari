// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotenvMissing(t *testing.T) {
	vals, existed, err := loadDotenv(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if existed {
		t.Error("existed = true for a missing file")
	}
	if vals != nil {
		t.Errorf("vals = %#v, want nil", vals)
	}
}

// TestLoadDotenv exercises the godotenv-backed parse through the file path,
// including a leading UTF-8 BOM (which godotenv alone rejects), a trailing
// comment, single/double quoting, and same-file ${VAR} expansion.
func TestLoadDotenv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "\ufeff" + `# a comment
PORT=9090
TOKEN="ab cd"      # trailing comment
LITERAL='raw $val'
REF=${PORT}-ref
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	vals, existed, err := loadDotenv(path)
	if err != nil || !existed {
		t.Fatalf("loadDotenv: existed=%v err=%v", existed, err)
	}
	want := map[string]string{
		"PORT":    "9090",
		"TOKEN":   "ab cd",
		"LITERAL": "raw $val",
		"REF":     "9090-ref",
	}
	for k, v := range want {
		if vals[k] != v {
			t.Errorf("%s = %q, want %q", k, vals[k], v)
		}
	}
}

// TestLayeredLookupPrecedence pins process env > .env > env: default by driving
// resolveEnv through the composed lookup, and confirms the env: allowlist gate
// still drops keys the .env supplies but the project does not declare.
func TestLayeredLookupPrecedence(t *testing.T) {
	decls := map[string]any{
		"FROM_PROC":    nil,
		"FROM_DOTENV":  nil,
		"FROM_DEFAULT": "default-val",
		"OVERRIDDEN":   "default-val",
	}
	proc := map[string]string{"FROM_PROC": "proc", "OVERRIDDEN": "proc-wins"}
	dotenv := map[string]string{
		"FROM_DOTENV": "dotenv",
		"OVERRIDDEN":  "dotenv-loses",
		"UNDECLARED":  "ignored", // not in decls → never read
	}
	lookup := layeredLookup(func(k string) (string, bool) { v, ok := proc[k]; return v, ok }, dotenv)

	got, err := resolveEnv(decls, lookup)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"FROM_PROC":    "proc",
		"FROM_DOTENV":  "dotenv",
		"FROM_DEFAULT": "default-val",
		"OVERRIDDEN":   "proc-wins",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[k], v)
		}
	}
	if _, ok := got["UNDECLARED"]; ok {
		t.Error("UNDECLARED leaked past the env: allowlist")
	}
}
