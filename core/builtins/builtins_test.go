// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package builtins

import (
	"strings"
	"testing"
)

func TestExtractOperator(t *testing.T) {
	op, operand, err := ExtractOperator(map[string]any{"of": "x", "equals": 1})
	if err != nil || op != "equals" || operand != 1 {
		t.Fatalf("%v %v %v", op, operand, err)
	}
	if _, _, err := ExtractOperator(map[string]any{"of": "x"}); err == nil {
		t.Error("missing operator must error")
	}
	if _, _, err := ExtractOperator(map[string]any{"equals": 1, "gt": 0}); err == nil {
		t.Error("two operators must error")
	}
}

func TestCheckOperators(t *testing.T) {
	cases := []struct {
		of      any
		op      string
		operand any
		want    bool
	}{
		{"1", "equals", 1, true}, // numeric coercion
		{"abc", "equals", "abc", true},
		{"abc", "notEquals", "abd", true},
		{"hello world", "contains", "world", true},
		{[]any{"a", "b"}, "contains", "b", true},
		{"hello", "absent", "x", true},
		{"RUNNING", "in", []any{"RUNNING", "DONE"}, true},
		{"FAILED", "in", []any{"RUNNING", "DONE"}, false},
		{"stream started ok", "matches", "stream\\s+started", true},
		{5, "gt", 4, true},
		{5, "lt", 4, false},
		{5, "gte", 5, true},
		{5, "lte", 4, false},
		{5, "between", []any{1, 10}, true},
		{50, "between", []any{1, 10}, false},
		{nil, "equals", nil, true},     // explicit null asserts absence
		{nil, "equals", "", false},     // a typo'd reference must not pass equals ""
		{"", "equals", nil, false},     // and the mirror image
		{nil, "notEquals", nil, false}, // null is null
	}
	for _, c := range cases {
		got, msg, err := Check(c.of, c.op, c.operand)
		if err != nil {
			t.Errorf("%v %s %v: %v", c.of, c.op, c.operand, err)
			continue
		}
		if got != c.want {
			t.Errorf("%v %s %v = %v (want %v) msg=%s", c.of, c.op, c.operand, got, c.want, msg)
		}
	}
}

func TestCheckBadOperands(t *testing.T) {
	if _, _, err := Check("x", "in", "notalist"); err == nil {
		t.Error("in with non-list must error")
	}
	if _, _, err := Check("x", "matches", "("); err == nil {
		t.Error("invalid regex must error")
	}
	if _, _, err := Check("abc", "gt", "def"); err == nil {
		t.Error("gt on non-numbers must error")
	}
	if _, _, err := Check(5, "between", []any{10, 1}); err == nil || !strings.Contains(err.Error(), "reversed") {
		t.Errorf("reversed between bounds must error, got %v", err)
	}
	if _, _, err := Check(map[string]any{"a": 1}, "contains", "a:1"); err == nil {
		t.Error("contains on a map must error, not substring-match its fmt rendering")
	}
}

func TestAssertOfIsRequired(t *testing.T) {
	for _, a := range Specs()["assert"].Args {
		if a.Name == "of" {
			if !a.Required {
				t.Fatal("assert's `of` must be required — a missing of compares nil and passes vacuously")
			}
			return
		}
	}
	t.Fatal("assert spec has no `of` arg")
}

func TestSpecsCoverLanguage(t *testing.T) {
	specs := Specs()
	for _, name := range []string{"assert", "sleep", "wait_until", "background", "stop_background"} {
		if _, ok := specs[name]; !ok {
			t.Errorf("missing builtin %s", name)
		}
	}
	if !strings.Contains(strings.Join(Operators, ","), "notEquals") {
		t.Error("operator set must be the closed set")
	}
}
