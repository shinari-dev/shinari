// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tagexpr

import "testing"

func TestCompileAndEval(t *testing.T) {
	cases := []struct {
		expr string
		tags []string
		want bool
	}{
		{"slow", []string{"slow"}, true},
		{"slow", []string{"fast"}, false},
		{"slow & redis", []string{"slow", "redis"}, true},
		{"slow & redis", []string{"slow"}, false},
		{"slow | redis", []string{"redis"}, true},
		{"!flaky", []string{"slow"}, true},
		{"!flaky", []string{"flaky"}, false},
		{"(slow | fast) & !wip", []string{"fast"}, true},
		{"(slow | fast) & !wip", []string{"fast", "wip"}, false},
		{"a & b | c", []string{"c"}, true},  // | is lowest precedence
		{"a & b | c", []string{"a"}, false}, // a alone does not satisfy a&b
		{"net.core/slow-1", []string{"net.core/slow-1"}, true},
	}
	for _, tc := range cases {
		e, err := Compile(tc.expr)
		if err != nil {
			t.Fatalf("Compile(%q): unexpected error %v", tc.expr, err)
		}
		if got := e.Eval(tc.tags); got != tc.want {
			t.Errorf("Compile(%q).Eval(%v) = %v, want %v", tc.expr, tc.tags, got, tc.want)
		}
	}
}

func TestCompileErrors(t *testing.T) {
	for _, expr := range []string{"", "slow &", "& slow", "(slow", "slow )", "slow @ redis", "()", "!"} {
		if _, err := Compile(expr); err == nil {
			t.Errorf("Compile(%q): expected error, got nil", expr)
		}
	}
}
