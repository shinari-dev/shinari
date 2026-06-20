// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package jqx

import (
	"reflect"
	"sort"
	"testing"
)

func TestRootRefs(t *testing.T) {
	cases := []struct {
		expr string
		want []string
	}{
		{".state", []string{"state"}},
		{".rsp.value.total", []string{"rsp"}}, // only the root, not nested fields
		{".runs | length", []string{"runs"}},
		{".total // 0", []string{"total"}},
		{".a + .b", []string{"a", "b"}},
		{`"job-\(.id)"`, []string{"id"}},
		{".items[0].id", []string{"items"}},
	}
	for _, c := range cases {
		got := RootRefs(c.expr)
		sort.Strings(got)
		want := append([]string(nil), c.want...)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("RootRefs(%q) = %v, want %v", c.expr, got, want)
		}
	}
}

func TestNSRefs(t *testing.T) {
	cases := []struct {
		expr string
		want []Ref
	}{
		{".vars.region", []Ref{{"vars", "region"}}},
		{".outputs.rsp.value.total", []Ref{{"outputs", "rsp"}}},
		{".env.DATABASE_URL // \"x\"", []Ref{{"env", "DATABASE_URL"}}},
		{"\"${.params.target}-${.vars.n}\"", []Ref{{"params", "target"}, {"vars", "n"}}},
		{".foo", []Ref{{"foo", ""}}},
		{"length", nil},
	}
	for _, c := range cases {
		got := NSRefs(c.expr)
		if len(got) != len(c.want) {
			t.Fatalf("NSRefs(%q) = %v, want %v", c.expr, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("NSRefs(%q)[%d] = %v, want %v", c.expr, i, got[i], c.want[i])
			}
		}
	}
}
