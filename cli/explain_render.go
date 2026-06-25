// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/core/registry"
)

// explainScenario writes one scenario's explain block: header, metadata, and
// the lifecycle sections. Shared by the `explain` command and the TUI.
func explainScenario(w io.Writer, set *discover.Set, sc *model.Scenario) {
	fmt.Fprintf(w, "=== %s\n", sc.Name)
	if sc.Description != "" {
		fmt.Fprintf(w, "%s\n", strings.TrimSpace(sc.Description))
	}
	meta := []string{}
	if len(sc.Tags) > 0 {
		meta = append(meta, "tags: "+strings.Join(sc.Tags, ", "))
	}
	if sc.Timeout > 0 {
		meta = append(meta, fmt.Sprintf("timeout: %gs", sc.Timeout))
	}
	if len(meta) > 0 {
		fmt.Fprintf(w, "%s\n", strings.Join(meta, "   "))
	}

	reg, rerr := registry.New(set, model.MergeProviders(set.Project.Providers, sc.Providers))
	if rerr != nil {
		fmt.Fprintf(w, "  provider configuration error: %v\n", rerr)
		return
	}
	explainSteps(w, reg, "setup", sc.Setup)
	if len(sc.SteadyState) > 0 {
		explainSteps(w, reg, "steadyState (gate, then recovery after method)", sc.SteadyState)
	}
	for _, ph := range sc.Method {
		fmt.Fprintf(w, "method — %s:\n", ph.Phase)
		explainStepLines(w, reg, ph.Steps)
	}
	explainSteps(w, reg, "verify", sc.Verify)
	explainTeardown(w, reg, sc)
}

// explainString renders explainScenario to a string for the TUI.
func explainString(set *discover.Set, sc *model.Scenario) string {
	var b strings.Builder
	explainScenario(&b, set, sc)
	return b.String()
}
