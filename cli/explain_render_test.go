// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/shinari-dev/shinari/core/discover"
)

func explainSet(t *testing.T) *discover.Set {
	t.Helper()
	dir := writeFindingProject(t) // project.yml + s.yml (scenario "known-gap")
	set, err := discover.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestExplainStringRendersTimeline(t *testing.T) {
	set := explainSet(t)
	sc := set.Scenarios[0]
	out := explainString(set, sc)
	for _, want := range []string{sc.Name, "verify"} {
		if !strings.Contains(out, want) {
			t.Fatalf("explainString missing %q in:\n%s", want, out)
		}
	}
}
