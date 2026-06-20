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
