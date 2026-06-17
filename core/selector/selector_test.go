// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package selector

import (
	"strings"
	"testing"

	"github.com/shinari-dev/shinari/core/model"
)

func scn(name string, tags ...string) *model.Scenario {
	sc := &model.Scenario{Tags: tags}
	sc.Name = name
	return sc
}

func names(scs []*model.Scenario) []string {
	out := make([]string, 0, len(scs))
	for _, sc := range scs {
		out = append(out, sc.Name)
	}
	return out
}

func TestFilter(t *testing.T) {
	all := []*model.Scenario{
		scn("a", "slow", "redis"),
		scn("b", "fast"),
		scn("c", "slow", "flaky"),
		scn("d"),
	}
	cases := []struct {
		name    string
		include string
		exclude string
		want    string
	}{
		{"no filter keeps all", "", "", "a,b,c,d"},
		{"include or", "slow | fast", "", "a,b,c"},
		{"include and", "slow & redis", "", "a"},
		{"exclude only", "", "flaky", "a,b,d"},
		{"include then exclude wins", "slow", "flaky", "a"},
		{"include matches none", "missing", "", ""},
		{"not expression", "!slow", "", "b,d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Filter(all, tc.include, tc.exclude)
			if err != nil {
				t.Fatalf("Filter: %v", err)
			}
			if g := strings.Join(names(got), ","); g != tc.want {
				t.Errorf("got %q, want %q", g, tc.want)
			}
		})
	}
}

func TestFilterBadExpr(t *testing.T) {
	if _, err := Filter(nil, "slow &", ""); err == nil || !strings.Contains(err.Error(), "include-tags") {
		t.Fatalf("include error = %v, want one mentioning include-tags", err)
	}
	if _, err := Filter(nil, "", "(bad"); err == nil || !strings.Contains(err.Error(), "exclude-tags") {
		t.Fatalf("exclude error = %v, want one mentioning exclude-tags", err)
	}
}
