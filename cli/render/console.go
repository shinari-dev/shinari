// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package render turns the core Result/event contract into every CLI
// output: console stream, results.tsv, results.json, junit.xml, JSON
// journal, findings report. Rendering is exclusively a front-end concern
// — nothing here reaches into engine internals.
package render

import (
	"fmt"
	"io"

	"github.com/shinari-dev/shinari/core/engine"
)

// Console is a streaming renderer: it subscribes to the live event stream.
type Console struct {
	W io.Writer
}

func (c *Console) Emit(e engine.Event) {
	switch e.Type {
	case engine.EvScenarioStarted:
		fmt.Fprintf(c.W, "\n=== %s\n", e.Scenario)
	case engine.EvPhaseStarted:
		fmt.Fprintf(c.W, "  -- %s\n", e.Phase)
	case engine.EvStepPassed:
		if e.Payload["verdict"] == string(engine.CheckFinding) {
			return
		}
		fmt.Fprintf(c.W, "  ✓ %s\n", e.Step)
	case engine.EvStepFailed:
		if e.Payload["verdict"] == string(engine.CheckFinding) {
			fmt.Fprintf(c.W, "  ◆ FINDING %s\n", e.Step)
			return
		}
		fmt.Fprintf(c.W, "  ✗ %s — %v\n", e.Step, e.Payload["error"])
	case engine.EvStepSkipped:
		fmt.Fprintf(c.W, "  ↷ %s (skipped)\n", e.Step)
	case engine.EvFaultInjected:
		fmt.Fprintf(c.W, "  ⚡ fault injected: %s\n", e.Verb)
	case engine.EvGateObserved:
		fmt.Fprintf(c.W, "  ▷ gate observed: %v\n", e.Payload["observed"])
	case engine.EvScenarioFinished:
		fmt.Fprintf(c.W, "  => %v\n", e.Payload["verdict"])
	}
}

// Summary prints the run roll-up after the stream ends.
func Summary(w io.Writer, res engine.RunResult) {
	var passed, failed, errored, inconclusive, findings int
	for _, sc := range res.Scenarios {
		switch sc.Verdict {
		case engine.ScenarioPassed:
			passed++
		case engine.ScenarioFailed:
			failed++
		case engine.ScenarioErrored:
			errored++
		case engine.ScenarioInconclusive:
			inconclusive++
		}
		for _, f := range sc.Findings {
			if !f.NowPasses {
				findings++
			}
		}
	}
	fmt.Fprintf(w, "\n%d scenario(s): %d passed, %d failed, %d errored, %d inconclusive — %d finding(s) held\n",
		len(res.Scenarios), passed, failed, errored, inconclusive, findings)
	for _, sc := range res.Scenarios {
		if sc.Verdict != engine.ScenarioPassed {
			fmt.Fprintf(w, "  %s: %s — %s\n", sc.Verdict, sc.Name, sc.Reason)
		}
	}
}
