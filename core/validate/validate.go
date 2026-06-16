// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package validate implements the static checks. Each finding
// names file, scenario, step, and reason — fail fast and clear.
package validate

import (
	"fmt"
	"strings"

	"github.com/shinari-dev/shinari/core/builtins"
	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/interp"
	"github.com/shinari-dev/shinari/core/jqx"
	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/core/registry"
	"github.com/shinari-dev/shinari/sdk"
)

type Severity string

const (
	Error Severity = "error"
	Warn  Severity = "warn"
)

type Finding struct {
	File     string
	Scenario string
	Step     string // run: of the offending step
	Rule     int    // rule number
	Msg      string
	Severity Severity
}

func (f Finding) String() string {
	loc := f.File
	if f.Scenario != "" {
		loc += " scenario " + f.Scenario
	}
	if f.Step != "" {
		loc += " step " + f.Step
	}
	return fmt.Sprintf("[%s] rule %d: %s: %s", f.Severity, f.Rule, loc, f.Msg)
}

// Validate runs every rule over a discovered project. Rules 1 and the
// parse half of rule 2 (header recognition, reserved envelope keys) are
// enforced by discovery/parsing; their failures arrive as load errors
// before this runs.
func Validate(set *discover.Set) []Finding {
	var out []Finding
	for _, def := range set.Providers {
		out = append(out, validateComposedDef(def)...)
	}
	for _, sc := range set.Scenarios {
		out = append(out, validateScenario(set, sc)...)
	}
	return out
}

func validateScenario(set *discover.Set, sc *model.Scenario) []Finding {
	var out []Finding
	merged := model.MergeProviders(set.Project.Providers, sc.Providers)
	reg, err := registry.New(set, merged)
	if err != nil {
		rule := 3
		if strings.Contains(err.Error(), "one level") {
			rule = 4 // macro nesting >1 surfaces at registry build
		}
		return []Finding{{File: sc.File, Scenario: sc.Name, Rule: rule, Msg: err.Error(), Severity: Error}}
	}

	// rule 8 — exactly one lifecycle provider. Zero is a warn (a pure
	// http/exec suite is legitimate; documented deviation from the spec's
	// strict "exactly one"), several is an error.
	switch lc := reg.Lifecycle(); {
	case len(lc) > 1:
		out = append(out, Finding{File: sc.File, Scenario: sc.Name, Rule: 8, Severity: Error,
			Msg: fmt.Sprintf("several lifecycle providers configured (%s) — exactly one provider may implement up/down", strings.Join(lc, ", "))})
	case len(lc) == 0:
		out = append(out, Finding{File: sc.File, Scenario: sc.Name, Rule: 8, Severity: Warn,
			Msg: "no lifecycle provider (up/down) configured — no default teardown, no stack lifecycle"})
	}

	defined := map[string]bool{}
	for k := range set.Project.Vars {
		defined[k] = true
	}
	for k := range sc.Vars {
		defined[k] = true
	}
	laterBound := map[string]string{} // capture name -> binding verb (for rule 6/10 hints)
	for _, sec := range sc.Sections() {
		for _, st := range sec.Steps {
			for name := range bindings(&st) {
				laterBound[name] = st.Run
			}
			for _, branch := range decodeBranches(&st) {
				for i := range branch {
					for name := range bindings(&branch[i]) {
						laterBound[name] = "parallel"
					}
				}
			}
		}
	}

	type stepCtx struct {
		section string
		step    *model.Step
	}
	var ordered []stepCtx
	for _, sec := range sc.Sections() {
		for i := range sec.Steps {
			ordered = append(ordered, stepCtx{sec.Name, &sec.Steps[i]})
		}
	}

	methodHasOutage := false
	methodCapturesID := false
	methodHasDegradation := false
	observesLatency := false
	verifyAwaitsCapture := false
	verifyHasExactlyOnce := false
	verifyHasFinding := false
	bgRunning := map[string]bool{}

	for _, sctx := range ordered {
		st := sctx.step
		res, rerr := reg.Resolve(st.Run)
		if rerr != nil {
			// rule 3 — unless the step opted into tri-state SKIP.
			if st.OnAbsent != "skip" {
				out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: st.Run, Rule: 3,
					Msg: rerr.Error(), Severity: Error})
			}
			continue
		}

		kind := res.Spec.Kind
		if st.Kind != "" {
			kind = sdk.Kind(st.Kind)
		}

		// rules 2 & 5 — with: matches the arg spec, finding: only on assertions.
		raw := rawWith(st)
		out = append(out, perStepArgAndFinding(sc, res.Spec, st, kind, raw)...)

		// rule 12 — parallel branch structure, per-branch rule recursion, and
		// the cross-branch reference ban.
		if st.Run == "parallel" {
			branches := decodeBranches(st)
			if len(branches) == 0 {
				out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: "parallel", Rule: 12,
					Msg: "parallel: branches must be a non-empty list", Severity: Error})
			}
			perBranch := make([]map[string]bool, len(branches))
			for bi, branch := range branches {
				perBranch[bi] = branchBindings(branch)
			}
			for bi := range branches {
				branch := branches[bi]
				if len(branch) == 0 {
					out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: "parallel", Rule: 12,
						Msg: fmt.Sprintf("parallel: branch %d is empty", bi), Severity: Error})
					continue
				}
				for si := range branch {
					bst := &branch[si]
					bres, berr := reg.Resolve(bst.Run)
					if berr != nil {
						if bst.OnAbsent != "skip" {
							out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: bst.Run, Rule: 3,
								Msg: berr.Error(), Severity: Error})
						}
						continue
					}
					bkind := bres.Spec.Kind
					if bst.Kind != "" {
						bkind = sdk.Kind(bst.Kind)
					}
					// rules 2 & 5 — same per-step checks as a top-level step.
					out = append(out, perStepArgAndFinding(sc, bres.Spec, bst, bkind, rawWith(bst))...)
					// rule 12 — cross-branch reference: a ${name} bound only in a
					// sibling branch (not a var, not pre-block, not this branch).
					for _, ref := range refsOf(bst) {
						for _, name := range jqx.RootRefs(ref) {
							if defined[name] || perBranch[bi][name] {
								continue
							}
							inSibling := false
							for bj := range perBranch {
								if bj != bi && perBranch[bj][name] {
									inSibling = true
								}
							}
							if inSibling {
								out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: bst.Run, Rule: 12,
									Msg: fmt.Sprintf("${%s} is bound in a sibling parallel branch — concurrent branches have no ordering; reference it after the block", name), Severity: Error})
							}
						}
					}
					// branch faults feed the recovery/degradation rules (7, 11)
					if strings.HasPrefix(sctx.section, "method") {
						beff := bres.Spec.Effect
						if bst.Effect != "" {
							beff = sdk.Effect(bst.Effect)
						}
						if beff == sdk.EffectOutage {
							methodHasOutage = true
						}
						if beff == sdk.EffectDegradation {
							methodHasDegradation = true
						}
					}
				}
			}
		}

		// rule 9 — steadyState idempotency.
		if sctx.section == "steadyState" && kind == sdk.KindAction && res.Spec.SideEffects {
			out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: st.Run, Rule: 9,
				Msg: "steadyState re-runs after method — a one-shot mutating verb here is not idempotent", Severity: Warn})
		}

		// rules 6 & 10 — references resolve, in execution order. Each ${...} is
		// a jq expression; check the top-level input fields it reads. Branch
		// references are checked per-branch in the parallel block below, not
		// here (the flat scope would mis-resolve them).
		if st.Run != "parallel" {
			for _, ref := range refsOf(st) {
				for _, name := range jqx.RootRefs(ref) {
					if defined[name] {
						continue
					}
					if binder, later := laterBound[name]; later {
						rule, msg := 10, fmt.Sprintf("${%s} is referenced before %s binds it", name, binder)
						if binder == "stop_background" || bgRunning[name] {
							rule = 6
							msg = fmt.Sprintf("${%s} is a background capture — it settles only at stop_background; reference it after", name)
						}
						out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: st.Run, Rule: rule,
							Msg: msg, Severity: Error})
						continue
					}
					out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: st.Run, Rule: 10,
						Msg: fmt.Sprintf("unresolved reference ${%s} — no var and no earlier capture by that name", name), Severity: Error})
				}
			}
		}

		// track state for later rules
		if st.Run == "background" {
			if m, ok := raw.(map[string]any); ok {
				if n, _ := m["name"].(string); n != "" {
					bgRunning[n] = true
				}
			}
		}
		for name := range bindings(st) {
			defined[name] = true
		}
		for _, branch := range decodeBranches(st) {
			for i := range branch {
				for name := range bindings(&branch[i]) {
					defined[name] = true
				}
			}
		}

		if st.Run == "sample" {
			observesLatency = true
		}
		for _, ref := range refsOf(st) {
			if strings.Contains(ref, "meta.durationMs") {
				observesLatency = true
			}
		}

		if strings.HasPrefix(sctx.section, "method") {
			// An outage-class fault (work can be lost) is what makes a
			// scenario recovery-shaped — declared by the verb (or by the step,
			// for faults injected via exec.run/http.post), not matched against
			// a hardcoded name list, so third-party faults count too.
			effect := res.Spec.Effect
			if st.Effect != "" {
				effect = sdk.Effect(st.Effect)
			}
			if effect == sdk.EffectOutage {
				methodHasOutage = true
			}
			if effect == sdk.EffectDegradation {
				methodHasDegradation = true
			}
			if kind == sdk.KindAction && (st.As != "" || len(st.Capture) > 0) {
				methodCapturesID = true
			}
		}
		if sctx.section == "verify" {
			if len(refsOf(st)) > 0 {
				verifyAwaitsCapture = true
			}
			if st.Finding != "" {
				verifyHasFinding = true
			}
			if st.Run == "assert" {
				if m, ok := raw.(map[string]any); ok {
					if v, has := m["equals"]; has && fmt.Sprintf("%v", v) == "1" {
						verifyHasExactlyOnce = true
					}
				}
			}
		}
	}

	// rule 7 — recovery invariant present.
	if methodHasOutage && methodCapturesID && !verifyHasExactlyOnce && !verifyHasFinding {
		sev, msg := Warn, "recovery-shaped scenario (fault + captured work): consider an exactly-once assertion (count equals: 1) or a finding:"
		if verifyAwaitsCapture {
			sev = Error
			msg = "recovery-shaped scenario (fault injected, work captured, verify awaits it) MUST assert exactly-once (count equals: 1) or carry a finding:"
		}
		out = append(out, Finding{File: sc.File, Scenario: sc.Name, Rule: 7, Msg: msg, Severity: sev})
	}

	// rule 11 — a degradation fault that nothing observes is a smell.
	if methodHasDegradation && !observesLatency {
		out = append(out, Finding{File: sc.File, Scenario: sc.Name, Rule: 11, Severity: Warn,
			Msg: "degradation fault injected but nothing observes it — assert a latency (${...meta.durationMs}) or use sample"})
	}

	return out
}

// validateComposedDef checks a kind: Provider body: every ${ref} in a verb
// body must reference a param or an earlier body capture. Composed verbs
// declare their inputs as params rather than reaching into caller vars.
func validateComposedDef(def *model.ProviderDef) []Finding {
	var out []Finding
	for verb, cv := range def.Verbs {
		names, _ := cv.ParamNames()
		known := map[string]bool{}
		for _, n := range names {
			known[n] = true
		}
		steps := cv.Do
		if cv.Probe != nil {
			steps = []model.Step{*cv.Probe}
		}
		for i := range steps {
			st := &steps[i]
			for _, ref := range refsOf(st) {
				for _, name := range jqx.RootRefs(ref) {
					if known[name] {
						continue
					}
					out = append(out, Finding{File: def.File, Step: st.Run, Rule: 10, Severity: Error,
						Msg: fmt.Sprintf("provider %s verb %s: ${%s} is neither a param nor an earlier capture", def.Name, verb, name)})
				}
			}
			for name := range bindings(st) {
				known[name] = true
			}
		}
	}
	return out
}

func bindings(st *model.Step) map[string]bool {
	out := map[string]bool{}
	if st.As != "" {
		out[st.As] = true
	}
	for name := range st.Capture {
		out[name] = true
	}
	return out
}

func rawWith(st *model.Step) any {
	raw, _ := st.DecodeWith()
	return raw
}

// perStepArgAndFinding runs the per-step checks shared by top-level steps and
// parallel branch steps: rule 2 (with: matches the arg spec, plus the assert/
// wait_until operator) and rule 5 (finding: only on assertion-kind checks).
func perStepArgAndFinding(sc *model.Scenario, spec sdk.VerbSpec, st *model.Step, kind sdk.Kind, raw any) []Finding {
	var out []Finding
	if _, err := registry.BindArgs(spec, raw); err != nil {
		out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: st.Run, Rule: 2,
			Msg: err.Error(), Severity: Error})
	}
	if st.Run == "assert" || st.Run == "wait_until" {
		if m, ok := raw.(map[string]any); ok {
			if _, _, oerr := builtins.ExtractOperator(m); oerr != nil {
				out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: st.Run, Rule: 2,
					Msg: oerr.Error(), Severity: Error})
			}
		}
	}
	if st.Finding != "" && kind != sdk.KindAssertion {
		out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: st.Run, Rule: 5,
			Msg: fmt.Sprintf("finding: is only allowed on assertion-kind checks; %s is kind %s", st.Run, kind), Severity: Error})
	}
	return out
}

// decodeBranches returns the branch step-lists of a parallel step, or nil if
// the step is not a well-formed parallel block.
func decodeBranches(st *model.Step) [][]model.Step {
	if st.Run != "parallel" {
		return nil
	}
	var w struct {
		Branches [][]model.Step `yaml:"branches"`
	}
	if err := st.With.Decode(&w); err != nil {
		return nil
	}
	return w.Branches
}

// branchBindings returns the set of capture names a single branch binds.
func branchBindings(branch []model.Step) map[string]bool {
	out := map[string]bool{}
	for i := range branch {
		for name := range bindings(&branch[i]) {
			out[name] = true
		}
	}
	return out
}

func refsOf(st *model.Step) []string {
	var refs []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			refs = append(refs, interp.Refs(t)...)
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(rawWith(st))
	return refs
}
