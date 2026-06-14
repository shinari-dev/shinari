// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"
)

func scope() Scope {
	return Scope{
		Vars:     map[string]any{"sleepSecs": 30, "name": "demo"},
		Captures: map[string]any{"job": "j-42", "total": float64(1), "obj": map[string]any{"id": "x"}},
	}
}

func TestVarsAndCaptures(t *testing.T) {
	s, err := scope().String("sleep ${vars.sleepSecs}s for ${job}")
	if err != nil || s != "sleep 30s for j-42" {
		t.Fatalf("got %q, %v", s, err)
	}
}

func TestUnresolvedIsErrorNamingRef(t *testing.T) {
	_, err := scope().String("${nope}")
	if err == nil || !strings.Contains(err.Error(), "${nope}") {
		t.Fatalf("want error naming ${nope}, got %v", err)
	}
}

func TestNoArithmetic(t *testing.T) {
	_, err := scope().String("${sleepSecs - 1}")
	if err == nil || !strings.Contains(err.Error(), "no expression language") {
		t.Fatalf("arithmetic must be an unresolved-ref error, got %v", err)
	}
}

func TestWholeValuePreservesType(t *testing.T) {
	v, err := scope().Value("${obj}")
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := v.(map[string]any); !ok || m["id"] != "x" {
		t.Fatalf("whole-value ref must keep the map, got %T %v", v, v)
	}
	s, err := scope().Value("id=${obj}")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(string); !ok {
		t.Fatalf("embedded ref must stringify, got %T", s)
	}
}

func TestCaptureShadowsVar(t *testing.T) {
	sc := Scope{Vars: map[string]any{"x": "var"}, Captures: map[string]any{"x": "cap"}}
	s, _ := sc.String("${x}")
	if s != "cap" {
		t.Fatalf("capture must win, got %q", s)
	}
	s, _ = sc.String("${vars.x}")
	if s != "var" {
		t.Fatalf("vars. prefix must read vars, got %q", s)
	}
}

func TestAnyWalksNested(t *testing.T) {
	v, err := scope().Any(map[string]any{"path": "/jobs/${job}", "list": []any{"${vars.name}", 7}})
	if err != nil {
		t.Fatal(err)
	}
	m := v.(map[string]any)
	if m["path"] != "/jobs/j-42" || m["list"].([]any)[0] != "demo" || m["list"].([]any)[1] != 7 {
		t.Fatalf("got %v", m)
	}
}

func TestRefs(t *testing.T) {
	got := Refs("a ${x} b ${vars.y}")
	if len(got) != 2 || got[0] != "x" || got[1] != "vars.y" {
		t.Fatalf("got %v", got)
	}
}
