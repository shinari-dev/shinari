// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/core/registry"
)

// Run executes the targeted scenarios of a discovered project. A target is
// a scenario name or a suite; empty targets means all. Providers are
// merged per scenario (project defaults + scenario overrides).
func Run(ctx context.Context, set *discover.Set, targets []string, em Emitter, opts Options) (RunResult, error) {
	scenarios, err := selectScenarios(set, targets)
	if err != nil {
		return RunResult{}, err
	}
	result := RunResult{Start: opts.now()}
	for _, sc := range scenarios {
		merged := model.MergeProviders(set.Project.Providers, sc.Providers)
		reg, rerr := registry.New(set, merged)
		if rerr != nil {
			result.Scenarios = append(result.Scenarios, ScenarioResult{
				Name: sc.Name, Suite: sc.Suite, Verdict: ScenarioErrored,
				Reason: "provider configuration: " + rerr.Error(),
				Start:  opts.now(), End: opts.now(),
			})
			continue
		}
		result.Scenarios = append(result.Scenarios,
			RunScenario(ctx, sc, set.Project.Vars, reg, em, opts))
	}
	result.End = opts.now()
	return result, nil
}

func selectScenarios(set *discover.Set, targets []string) ([]*model.Scenario, error) {
	if len(targets) == 0 {
		return set.Scenarios, nil
	}
	var out []*model.Scenario
	seen := map[string]bool{}
	for _, target := range targets {
		matched := false
		for _, sc := range set.Scenarios {
			if sc.Name == target || sc.Suite == target {
				matched = true
				if !seen[sc.Name] {
					seen[sc.Name] = true
					out = append(out, sc)
				}
			}
		}
		if !matched {
			var known []string
			for _, sc := range set.Scenarios {
				known = append(known, sc.Name)
			}
			sort.Strings(known)
			return nil, fmt.Errorf("no scenario or suite named %q (known scenarios: %s)", target, strings.Join(known, ", "))
		}
	}
	return out, nil
}

// Reduce rebuilds the scenario skeleton (names, step verdicts, scenario
// verdicts) from an event stream — the design constraint that Result is the
// stream's deterministic reduction. Renderers needing only this shape can
// work from a journal alone.
func Reduce(events []Event) RunResult {
	var run RunResult
	byName := map[string]*ScenarioResult{}
	order := []string{}
	for _, e := range events {
		switch e.Type {
		case EvScenarioStarted:
			byName[e.Scenario] = &ScenarioResult{Name: e.Scenario, Start: e.Time}
			order = append(order, e.Scenario)
		case EvStepPassed, EvStepFailed, EvStepSkipped:
			sc := byName[e.Scenario]
			if sc == nil {
				continue
			}
			verdict := CheckVerdict(fmt.Sprintf("%v", e.Payload["verdict"]))
			errMsg, _ := e.Payload["error"].(string)
			sc.Steps = append(sc.Steps, StepResult{
				Section: e.Section, Phase: e.Phase, Run: e.Verb,
				Verdict: verdict, Err: errMsg, End: e.Time,
			})
		case EvFaultInjected:
			if sc := byName[e.Scenario]; sc != nil {
				sc.Injected = append(sc.Injected, e.Step)
			}
		case EvFindingRecorded:
			if sc := byName[e.Scenario]; sc != nil {
				narrative, _ := e.Payload["narrative"].(string)
				nowPasses, _ := e.Payload["nowPasses"].(bool)
				detail, _ := e.Payload["detail"].(string)
				sc.Findings = append(sc.Findings, FindingRecord{
					Scenario: e.Scenario, Narrative: narrative, Check: e.Step,
					Detail: detail, NowPasses: nowPasses,
				})
			}
		case EvScenarioFinished:
			if sc := byName[e.Scenario]; sc != nil {
				sc.Verdict = ScenarioVerdict(fmt.Sprintf("%v", e.Payload["verdict"]))
				sc.Reason, _ = e.Payload["reason"].(string)
				sc.End = e.Time
			}
		}
	}
	for _, name := range order {
		run.Scenarios = append(run.Scenarios, *byName[name])
	}
	if n := len(events); n > 0 {
		run.Start = events[0].Time
		run.End = events[n-1].Time
	}
	return run
}
