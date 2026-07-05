// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

func scope() Scope {
	return Scope{
		Vars:    map[string]any{"job": "sleep-1", "n": 3},
		Outputs: map[string]any{"rsp": map[string]any{"value": map[string]any{"total": 19.9}}},
		Env:     map[string]any{"REGION": "us-east-1"},
	}
}

func TestStringInterpolatesNamespacedJQ(t *testing.T) {
	got, err := scope().String("job is ${.vars.job} in ${.env.REGION}")
	if err != nil || got != "job is sleep-1 in us-east-1" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestValuePreservesTypeForSingleRef(t *testing.T) {
	v, err := scope().Value("${.outputs.rsp.value.total}")
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := v.(float64); !ok || f != 19.9 {
		t.Fatalf("v = %v (%T)", v, v)
	}
}

func TestVarAndOutputDoNotCollide(t *testing.T) {
	sc := Scope{Vars: map[string]any{"x": "var"}, Outputs: map[string]any{"x": "out"}}
	got, err := sc.String("${.vars.x}/${.outputs.x}")
	if err != nil || got != "var/out" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestMissingResolvesToNull(t *testing.T) {
	got, err := scope().String("x=${.vars.missing}")
	if err != nil || got != "x=" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestInvalidJQIsError(t *testing.T) {
	_, err := scope().String("${vars.job}") // missing leading dot is invalid jq
	if err == nil {
		t.Fatal("want error for invalid jq expression")
	}
}

func TestEscapedDollarBraceIsLiteral(t *testing.T) {
	// shell-style ${VAR} in an exec snippet: escape with a second dollar
	got, err := scope().String("echo $${HOME} for ${.vars.job}")
	if err != nil || got != "echo ${HOME} for sleep-1" {
		t.Fatalf("got %q err %v", got, err)
	}
	if refs := Refs("run $${PATH} then ${.vars.n}"); len(refs) != 1 || refs[0] != ".vars.n" {
		t.Fatalf("escaped literals must not count as references, got %v", refs)
	}
	// a whole-string escaped literal stays a string, not a jq eval
	v, err := scope().Value("$${HOME}")
	if err != nil || v != "${HOME}" {
		t.Fatalf("v = %v err %v", v, err)
	}
}

func TestNonScalarEmbedsAsJSON(t *testing.T) {
	sc := Scope{Vars: map[string]any{
		"m": map[string]any{"a": 1.0},
		"l": []any{"a", "b"},
	}}
	got, err := sc.String(`m is ${.vars.m} and l is ${.vars.l}`)
	if err != nil || got != `m is {"a":1} and l is ["a","b"]` {
		t.Fatalf("got %q err %v", got, err)
	}
}
