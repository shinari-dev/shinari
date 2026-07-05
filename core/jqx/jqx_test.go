// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package jqx

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEvalWithMetaVariable(t *testing.T) {
	// The value stays the `.` input; metadata arrives as $meta.
	v, err := EvalWith("$meta.status", map[string]any{"id": "j-1"},
		map[string]any{"$meta": map[string]any{"status": 401}})
	if err != nil || v != float64(401) {
		t.Fatalf("got %v (%T), %v", v, v, err)
	}
	// `.` is still the value, untouched by the bound variable.
	v, err = EvalWith(".id", map[string]any{"id": "j-1"},
		map[string]any{"$meta": map[string]any{"status": 200}})
	if err != nil || v != "j-1" {
		t.Fatalf("got %v, %v", v, err)
	}
}

func TestEvalWithUndefinedVariableErrors(t *testing.T) {
	// A read: referencing a variable the engine did not bind must surface as a
	// jq error, not silently yield null.
	if _, err := EvalWith("$nope", nil, nil); err == nil {
		t.Fatal("want error for undefined variable, got nil")
	}
}

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

func TestNormalizeProviderShapedValues(t *testing.T) {
	// typed values a provider may hand back must not make gojq fail opaquely
	if v, err := Eval(". + 1", int32(3)); err != nil || v != float64(4) {
		t.Errorf("int32: got %v, %v", v, err)
	}
	if v, err := Eval(". + 1", uint(3)); err != nil || v != float64(4) {
		t.Errorf("uint: got %v, %v", v, err)
	}
	if v, err := Eval(". * 2", float32(1.5)); err != nil || v != float64(3) {
		t.Errorf("float32: got %v, %v", v, err)
	}
	if v, err := Eval(". * 2", json.Number("2.5")); err != nil || v != float64(5) {
		t.Errorf("json.Number: got %v, %v", v, err)
	}
	if v, err := Eval("length", []string{"a", "b"}); err != nil || v != 2 {
		t.Errorf("[]string: got %v, %v", v, err)
	}
	if v, err := Eval(".k", map[string]string{"k": "v"}); err != nil || v != "v" {
		t.Errorf("map[string]string: got %v, %v", v, err)
	}
	if _, err := Eval("length", time.Now()); err != nil {
		t.Errorf("time.Time must normalize to a string, got %v", err)
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
