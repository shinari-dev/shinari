// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package jqx

import (
	"strings"
	"testing"
)

func TestDotPath(t *testing.T) {
	v, err := Eval(".id", map[string]any{"id": "j-42"})
	if err != nil || v != "j-42" {
		t.Fatalf("got %v, %v", v, err)
	}
}

func TestPipeLength(t *testing.T) {
	v, err := Eval(".items | length", map[string]any{"items": []any{1, 2, 3}})
	if err != nil || v != 3 {
		t.Fatalf("got %v (%T), %v", v, v, err)
	}
}

func TestYAMLIntNormalized(t *testing.T) {
	v, err := Eval(".n", map[string]any{"n": 5})
	if err != nil || v != float64(5) {
		t.Fatalf("got %v (%T), %v", v, v, err)
	}
}

func TestInvalidExprNamesExpr(t *testing.T) {
	_, err := Eval(".[unclosed", nil)
	if err == nil || !strings.Contains(err.Error(), ".[unclosed") {
		t.Fatalf("want error naming the expr, got %v", err)
	}
}

func TestEnvBuiltinCannotReadOSEnv(t *testing.T) {
	t.Setenv("SHINARI_ENV_GUARD", "leaked")
	for _, expr := range []string{`env.SHINARI_ENV_GUARD`, `$ENV.SHINARI_ENV_GUARD`} {
		v, err := Eval(expr, nil)
		if err != nil {
			t.Fatalf("Eval(%q) error: %v", expr, err)
		}
		if v != nil {
			t.Fatalf("Eval(%q) = %#v, want nil — the bare env builtin must not read the OS environment", expr, v)
		}
	}
}
