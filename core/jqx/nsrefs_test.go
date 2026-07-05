// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package jqx

import (
	"reflect"
	"testing"
)

func TestNSRefs(t *testing.T) {
	cases := []struct {
		expr string
		want []Ref
	}{
		// navigation: the namespace plus its first field.
		{".vars.region", []Ref{{"vars", "region"}}},
		{".outputs.rsp.value.total", []Ref{{"outputs", "rsp"}}},
		{".foo", []Ref{{"foo", ""}}}, // single segment: caller flags it as non-namespaced
		{"length", nil},              // a function, no root field access
		{".", nil},                   // identity reads no field

		// predicates (when:): both sides of a non-pipe operator read the root.
		{".outputs.total.value > 1", []Ref{{"outputs", "total"}}},
		{".vars.n > 0", []Ref{{"vars", "n"}}},
		{".env.DATABASE_URL // \"x\"", []Ref{{"env", "DATABASE_URL"}}},
		{".outputs.a // .vars.b", []Ref{{"outputs", "a"}, {"vars", "b"}}},
		{".a + .b", []Ref{{"a", ""}, {"b", ""}}},

		// transforms: only fields that read the ROOT input are refs. A field
		// after a pipe, or inside a function argument, reads a rebound `.` and
		// must not be reported — the regex used to flag these as unresolved.
		{".outputs.runs | length", []Ref{{"outputs", "runs"}}},
		{".outputs.x | map(.state)", []Ref{{"outputs", "x"}}},
		{"[.outputs.runs[] | select(.failed)] | length", []Ref{{"outputs", "runs"}}},
		{".outputs.runs[]", []Ref{{"outputs", "runs"}}}, // iterator stops the path

		// distinct, in source order.
		{".vars.a + .vars.a + .vars.b", []Ref{{"vars", "a"}, {"vars", "b"}}},

		// refs inside if/object/unary/try/reduce/foreach read the root too —
		// they used to be invisible to the validator.
		{"if .vars.x then 1 else 2 end", []Ref{{"vars", "x"}}},
		{"if .vars.x then .outputs.a else .outputs.b end",
			[]Ref{{"vars", "x"}, {"outputs", "a"}, {"outputs", "b"}}},
		{"{a: .vars.x}", []Ref{{"vars", "x"}}},
		{"-.vars.n", []Ref{{"vars", "n"}}},
		{"try .vars.x catch .vars.y", []Ref{{"vars", "x"}, {"vars", "y"}}},
		{"reduce .outputs.runs[] as $r (0; . + 1)", []Ref{{"outputs", "runs"}}},
		{"foreach .outputs.runs[] as $r (0; . + 1)", []Ref{{"outputs", "runs"}}},
	}
	for _, c := range cases {
		got := NSRefs(c.expr)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("NSRefs(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}
