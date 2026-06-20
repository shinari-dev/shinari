// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/shinari-dev/shinari/core/model"
)

// maxLiveBranches caps concurrently-executing branches across the whole
// nesting tree. It is a safety limit, not a semantic restriction: branch sets
// are inline literals, so the concurrency tree is always static and finite.
const maxLiveBranches = 64

// runParallel executes a `parallel` step. Every branch runs concurrently on a
// cloned, buffered runner; at the barrier the branches are flushed back in
// branch-index order, so the event stream and the reduced result stay
// replay-deterministic despite concurrent execution. All branches always run
// to completion — no sibling cancellation — so outcomes never depend on race
// timing. A failing branch step fails the parallel step; a branch finding stays
// a finding and keeps the scenario green.
func (r *runner) runParallel(ctx context.Context, section, phase string, st *model.Step, finish func(CheckVerdict, string) StepResult) StepResult {
	var w struct {
		Branches [][]model.Step `yaml:"branches"`
	}
	if err := st.With.Decode(&w); err != nil {
		return finish(CheckFail, fmt.Sprintf("parallel: bad branches: %v", err))
	}
	if len(w.Branches) == 0 {
		return finish(CheckFail, "parallel: branches must be a non-empty list")
	}
	for i := range w.Branches {
		if len(w.Branches[i]) == 0 {
			return finish(CheckFail, fmt.Sprintf("parallel: branch %d is empty", i))
		}
		for j := range w.Branches[i] {
			w.Branches[i][j].File = st.File // inherit file for error messages
		}
	}

	runners := make([]*runner, len(w.Branches))
	failures := make([]string, len(w.Branches)) // per branch: runSection's failure message, "" if it held
	var wg sync.WaitGroup
	for i := range w.Branches {
		runners[i] = r.branchRunner()
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if n := r.limiter.Add(1); int(n) > maxLiveBranches {
				r.limiter.Add(-1)
				failures[i] = fmt.Sprintf("parallel: live-branch cap %d exceeded", maxLiveBranches)
				return
			}
			defer r.limiter.Add(-1)
			if msg, ok := runners[i].runSection(ctx, section, phase, w.Branches[i], true); !ok {
				failures[i] = msg
			}
		}(i)
	}
	wg.Wait()

	// Deterministic flush in branch-index order: replay each branch's events
	// and merge its results before moving to the next.
	var reasons []string
	for bi, br := range runners {
		for _, e := range br.emit.(*Recorder).Events {
			r.emit.Emit(e)
		}
		r.res.Steps = append(r.res.Steps, br.res.Steps...)
		r.res.Findings = append(r.res.Findings, br.res.Findings...)
		r.res.Injected = append(r.res.Injected, br.res.Injected...)
		r.res.Held = append(r.res.Held, br.res.Held...)
		for k := range br.writes {
			r.outputs[k] = br.outputs[k] // higher-indexed branch wins on collision
		}
		if failures[bi] != "" {
			reasons = append(reasons, failures[bi])
		}
	}
	if len(reasons) > 0 {
		return finish(CheckFail, strings.Join(reasons, "; "))
	}
	return finish(CheckPass, "")
}

// branchRunner clones the runner for one branch: shared read-mostly config
// (registry, options, vars, scenario, the live-branch limiter) but its own
// buffered emitter, result accumulator, capture copy + write-set, and
// background-handle map, so concurrent branches never share mutable state.
func (r *runner) branchRunner() *runner {
	caps := make(map[string]any, len(r.outputs))
	for k, v := range r.outputs {
		caps[k] = v
	}
	return &runner{
		reg:     r.reg,
		emit:    &Recorder{},
		sc:      r.sc,
		opts:    r.opts,
		outputs: caps,
		writes:  map[string]bool{},
		vars:    r.vars,
		env:     r.env,
		bg:      map[string]*bgHandle{},
		res:     &ScenarioResult{},
		limiter: r.limiter,
	}
}
