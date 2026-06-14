// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package conv

import "testing"

func TestToFloat(t *testing.T) {
	cases := []struct {
		in   any
		want float64
		ok   bool
	}{
		{float64(1.5), 1.5, true},
		{int(3), 3, true},
		{int64(4), 4, true},
		{"2.5", 2.5, true},
		{"  7 ", 7, true}, // trimmed
		{"nope", 0, false},
		{true, 0, false},
		{nil, 0, false},
	}
	for _, c := range cases {
		got, ok := ToFloat(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ToFloat(%#v) = (%v, %v), want (%v, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestToString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"hi", "hi"},
		{float64(2), "2"},     // whole float, no trailing zeros
		{float64(2.5), "2.5"}, // no exponent
		{int(3), "3"},
		{int64(4), "4"},
		{nil, ""},
		{true, "true"},
	}
	for _, c := range cases {
		if got := ToString(c.in); got != c.want {
			t.Errorf("ToString(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("hello", 10); got != "hello" {
		t.Errorf("under limit: got %q", got)
	}
	if got := Truncate("hello", 5); got != "hello" {
		t.Errorf("at limit: got %q", got)
	}
	if got := Truncate("hello world", 5); got != "hello..." {
		t.Errorf("over limit: got %q", got)
	}
}
