// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestResolveEnv(t *testing.T) {
	decls := map[string]any{
		"DATABASE_URL": nil,  // required
		"PORT":         8080, // default
		"REGION":       "us-west",
	}
	env := map[string]string{"DATABASE_URL": "postgres://x", "REGION": ""}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }

	got, err := resolveEnv(decls, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if got["DATABASE_URL"] != "postgres://x" {
		t.Fatalf("DATABASE_URL = %#v", got["DATABASE_URL"])
	}
	if got["PORT"] != 8080 {
		t.Fatalf("PORT = %#v, want 8080 (default)", got["PORT"])
	}
	if got["REGION"] != "" {
		t.Fatalf("REGION = %#v, want \"\" (set-but-empty beats default)", got["REGION"])
	}
}

func TestResolveEnvRequiredUnset(t *testing.T) {
	decls := map[string]any{"DATABASE_URL": nil}
	lookup := func(string) (string, bool) { return "", false }
	if _, err := resolveEnv(decls, lookup); err == nil {
		t.Fatal("want error for required-but-unset DATABASE_URL")
	}
}
