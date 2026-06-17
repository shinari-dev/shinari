// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"testing"

	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/model"
)

func taggedScn(name string, tags ...string) *model.Scenario {
	sc := &model.Scenario{Tags: tags}
	sc.Name = name
	return sc
}

// With both scenarios excluded, Run builds no registries and returns an
// empty result — proving the tag filter is applied before the run loop.
func TestRunExcludesByTag(t *testing.T) {
	set := &discover.Set{Scenarios: []*model.Scenario{
		taggedScn("a", "slow"),
		taggedScn("b", "fast"),
	}}
	res, err := Run(context.Background(), set, nil, &Recorder{}, Options{ExcludeTags: "slow | fast"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Scenarios) != 0 {
		t.Fatalf("got %d scenarios, want 0", len(res.Scenarios))
	}
}

func TestRunBadIncludeExpr(t *testing.T) {
	set := &discover.Set{Scenarios: []*model.Scenario{taggedScn("a", "slow")}}
	if _, err := Run(context.Background(), set, nil, &Recorder{}, Options{IncludeTags: "slow &"}); err == nil {
		t.Fatal("expected error for malformed include expression")
	}
}
