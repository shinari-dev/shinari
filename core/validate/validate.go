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

		// rule 2 — with: matches the verb's arg spec.
		raw := rawWith(st)
		if _, berr := registry.BindArgs(res.Spec, raw); berr != nil {
			out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: st.Run, Rule: 2,
				Msg: berr.Error(), Severity: Error})
		}
		if st.Run == "assert" || st.Run == "wait_until" {
			if m, ok := raw.(map[string]any); ok {
				if _, _, oerr := builtins.ExtractOperator(m); oerr != nil {
					out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: st.Run, Rule: 2,
						Msg: oerr.Error(), Severity: Error})
				}
			}
		}

		// rule 5 — finding: only on assertion-kind checks.
		if st.Finding != "" && kind != sdk.KindAssertion {
			out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: st.Run, Rule: 5,
				Msg: fmt.Sprintf("finding: is only allowed on assertion-kind checks; %s is kind %s", st.Run, kind), Severity: Error})
		}

		// rule 9 — steadyState idempotency.
		if sctx.section == "steadyState" && kind == sdk.KindAction && res.Spec.SideEffects {
			out = append(out, Finding{File: sc.File, Scenario: sc.Name, Step: st.Run, Rule: 9,
				Msg: "steadyState re-runs after method — a one-shot mutating verb here is not idempotent", Severity: Warn})
		}

		// rules 6 & 10 — references resolve, in execution order. Each ${...} is
		// a jq expression; check the top-level input fields it reads.
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
