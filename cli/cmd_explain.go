// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shinari-dev/shinari/core/engine"
	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/core/registry"
	"github.com/shinari-dev/shinari/sdk"
)

func newExplainCmd(project *string, stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "explain [target...]",
		Short: "preview a scenario's timeline without running it",
		Long: "Print what a scenario would do — its lifecycle timeline with each step's " +
			"resolved verb, kind, and fault effect — without executing anything or touching " +
			"the system. A target is a scenario name or a suite; no target means all.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := cmdExplain(*project, args, stdout, stderr); code != 0 {
				return &exitError{code}
			}
			return nil
		},
	}
}

func cmdExplain(dir string, targets []string, stdout, stderr io.Writer) int {
	set, ok := load(dir, stderr)
	if !ok {
		return 1
	}
	scenarios, err := engine.SelectScenarios(set, targets)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}

	for _, sc := range scenarios {
		fmt.Fprintln(stdout)
		explainScenario(stdout, set, sc)
	}
	fmt.Fprintf(stdout, "\nactions ([action]) are skipped under --dry-run; nothing here was executed.\n")
	return 0
}

// explainSteps prints a named section and its step lines (skipping empties).
func explainSteps(w io.Writer, reg *registry.Registry, section string, steps []model.Step) {
	if len(steps) == 0 {
		return
	}
	fmt.Fprintf(w, "%s:\n", section)
	explainStepLines(w, reg, steps)
}

func explainStepLines(w io.Writer, reg *registry.Registry, steps []model.Step) {
	for i := range steps {
		fmt.Fprintf(w, "  %s\n", explainLine(reg, &steps[i]))
	}
}

func explainTeardown(w io.Writer, reg *registry.Registry, sc *model.Scenario) {
	if sc.HasTeardown {
		explainSteps(w, reg, "teardown", sc.Teardown)
		return
	}
	// Default teardown mirrors the engine: <lifecycle>.down when exactly one
	// lifecycle provider is configured.
	if lc := reg.Lifecycle(); len(lc) == 1 {
		fmt.Fprintf(w, "teardown (default):\n  %s\n", explainLine(reg, &model.Step{Run: lc[0] + ".down"}))
	}
}

// explainLine renders one step: its verb, resolved kind, fault effect, and
// finding/description annotations. An unresolvable verb is flagged rather than
// failing — explain is a preview, not a gate (use validate for that).
func explainLine(reg *registry.Registry, st *model.Step) string {
	label := st.Run
	var tags []string
	res, err := reg.Resolve(st.Run)
	if err != nil {
		tags = append(tags, "[unresolved]")
	} else {
		tags = append(tags, fmt.Sprintf("[%s]", engine.EffectiveKind(res.Spec, st)))
		if eff := engine.EffectiveEffect(res.Spec, st); eff != sdk.EffectNone {
			tags = append(tags, fmt.Sprintf("⚡ fault: %s", eff))
		}
	}
	if st.When != "" {
		tags = append(tags, "when: "+st.When)
	}
	if st.Finding != "" {
		tags = append(tags, "◆ finding")
	}
	line := fmt.Sprintf("%-22s %s", label, strings.Join(tags, " "))
	if st.Desc != "" {
		line += fmt.Sprintf("  %q", st.Desc)
	}
	return strings.TrimRight(line, " ")
}
