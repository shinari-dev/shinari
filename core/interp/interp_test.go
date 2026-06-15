// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

func scope() Scope {
	return Scope{
		Vars:     map[string]any{"job": "sleep-1", "n": 3},
		Captures: map[string]any{"rsp": map[string]any{"value": map[string]any{"total": 19.9}}, "job": "override"},
	}
}

func TestStringInterpolatesJQ(t *testing.T) {
	got, err := scope().String("job is ${.job}")
	if err != nil || got != "job is override" { // capture shadows var
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestValuePreservesTypeForSingleRef(t *testing.T) {
	v, err := scope().Value("${.rsp.value.total}")
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := v.(float64); !ok || f != 19.9 {
		t.Fatalf("v = %v (%T)", v, v)
	}
}

func TestMissingResolvesToNull(t *testing.T) {
	got, err := scope().String("x=${.missing}")
	if err != nil || got != "x=" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestInvalidJQIsError(t *testing.T) {
	_, err := scope().String("${job}") // bare word is invalid jq
	if err == nil {
		t.Fatal("want error for invalid jq expression")
	}
}
