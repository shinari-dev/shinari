// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package validate implements the static checks. Each finding
// names file, scenario, step, and reason — fail fast and clear.
package validate

import (
	"fmt"
	"regexp"
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
	envSet := map[string]bool{}
	for k := range set.Project.Env {
		envSet[k] = true
	}
	for _, def := range set.Providers {
		out = append(out, validateComposedDef(def, envSet)...)
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

	varsSet := map[string]bool{}
	for k := range set.Project.Vars {
		varsSet[k] = true
	}
	for k := range sc.Vars {
		varsSet[k] = true
	}
	envSet := map[string]bool{}
	for k := range set.Project.Env {
		envSet[k] = true
	}
	defined := map[string]bool{} // outputs (captures), grown in execution order
	// Pre-scan every binding (recursively, including inside parallel branches)
	// so a forward/out-of-order reference can be told apart from a typo.
	laterBound := map[string]string{}
	for _, sec := range sc.Sections() {
		collectBindings(sec.Steps, func(name, verb string) { laterBound[name] = verb })
	}

	v := &scenarioValidator{sc: sc, reg: reg, laterBound: laterBound, bgRunning: map[string]bool{}, varsSet: varsSet, envSet: envSet}
	for _, sec := range sc.Sections() {
		for i := range sec.Steps {
			v.checkStep(&sec.Steps[i], sec.Name, defined, nil, 0)
		}
	}
	out = append(out, v.out...)

	// rule 7 — recovery invariant present.
	if v.methodHasOutage && v.methodCapturesID && !v.verifyHasExactlyOnce && !v.verifyHasFinding {
		sev, msg := Warn, "recovery-shaped scenario (fault + captured work): consider an exactly-once assertion (count equals: 1) or a finding:"
		if v.verifyAwaitsCapture {
			sev = Error
			msg = "recovery-shaped scenario (fault injected, work captured, verify awaits it) MUST assert exactly-once (count equals: 1) or carry a finding:"
		}
		out = append(out, Finding{File: sc.File, Scenario: sc.Name, Rule: 7, Msg: msg, Severity: sev})
	}

	// rule 11 — a degradation fault that nothing observes is a smell.
	if v.methodHasDegradation && !v.observesLatency {
		out = append(out, Finding{File: sc.File, Scenario: sc.Name, Rule: 11, Severity: Warn,
			Msg: "degradation fault injected but nothing observes it — assert a latency (${...meta.durationMs}) or use sample"})
	}

	out = append(out, validateTags(sc)...)

	return out
}

// tagPattern is the legal tag shape: anything else cannot appear in a tag
// expression (rule 14).
var tagPattern = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

// validateTags enforces rule 14: every tag must be expression-safe, and a
// repeated tag is a smell.
func validateTags(sc *model.Scenario) []Finding {
	var out []Finding
	seen := map[string]bool{}
	for _, tag := range sc.Tags {
		if !tagPattern.MatchString(tag) {
			out = append(out, Finding{File: sc.File, Scenario: sc.Name, Rule: 14, Severity: Error,
				Msg: fmt.Sprintf("tag %q has characters outside [A-Za-z0-9_./-]; it would be unparseable in a tag expression", tag)})
		}
		if seen[tag] {
			out = append(out, Finding{File: sc.File, Scenario: sc.Name, Rule: 14, Severity: Warn,
				Msg: fmt.Sprintf("duplicate tag %q", tag)})
		}
		seen[tag] = true
	}
	return out
}

// scenarioValidator carries the per-scenario reference scope helpers and the
// scenario-global accumulators rules 7 and 11 read, so the per-step checks run
// uniformly over top-level steps and parallel branch steps (the latter
// recursively, against a branch-local capture scope).
type scenarioValidator struct {
	sc         *model.Scenario
	reg        *registry.Registry
	laterBound map[string]string
	bgRunning  map[string]bool
	varsSet    map[string]bool // declared vars (project + scenario)
	envSet     map[string]bool // declared env: names (project)
	out        []Finding

	methodHasOutage      bool
	methodCapturesID     bool
	methodHasDegradation bool
	observesLatency      bool
	verifyAwaitsCapture  bool
	verifyHasExactlyOnce bool
	verifyHasFinding     bool
}

func (v *scenarioValidator) add(f Finding) {
	f.File = v.sc.File
	f.Scenario = v.sc.Name
	v.out = append(v.out, f)
}

// checkStep validates one step against the capture scope `defined`, which it
// extends with the step's own bindings. siblings/selfIdx carry the per-branch
// binding sets when the step runs inside a parallel branch (nil at the top
// level): a reference to a sibling branch's capture is rule 12 rather than a
// generic unresolved reference.
func (v *scenarioValidator) checkStep(st *model.Step, section string, defined map[string]bool, siblings []map[string]bool, selfIdx int) {
	res, rerr := v.reg.Resolve(st.Run)
	if rerr != nil {
		// rule 3 — unless the step opted into tri-state SKIP.
		if st.OnAbsent != "skip" {
			v.add(Finding{Step: st.Run, Rule: 3, Msg: rerr.Error(), Severity: Error})
		}
		return
	}
	kind := res.Spec.Kind
	if st.Kind != "" {
		kind = sdk.Kind(st.Kind)
	}
	raw := rawWith(st)

	// rules 2 & 5 — with: matches the arg spec, finding: only on assertions.
	v.out = append(v.out, perStepArgAndFinding(v.sc, res.Spec, st, kind, raw)...)

	if st.Run == "parallel" {
		v.checkParallel(st, section, defined)
		return
	}
	if st.Run == "repeat" {
		v.checkRepeat(st, section, defined)
		return
	}

	// rule 9 — steadyState idempotency.
	if section == "steadyState" && kind == sdk.KindAction && res.Spec.SideEffects {
		v.add(Finding{Step: st.Run, Rule: 9,
			Msg: "steadyState re-runs after method — a one-shot mutating verb here is not idempotent", Severity: Warn})
	}

	// rules 6, 10 & 12 — references resolve, by namespace, in execution order.
	for _, ref := range refsOf(st) {
		for _, r := range jqx.NSRefs(ref) {
			switch r.Namespace {
			case "vars":
				if !v.varsSet[r.Name] {
					v.add(Finding{Step: st.Run, Rule: 10,
						Msg: fmt.Sprintf("unresolved reference ${.vars.%s} — no var by that name", r.Name), Severity: Error})
				}
			case "env":
				if !v.envSet[r.Name] {
					v.add(Finding{Step: st.Run, Rule: 10,
						Msg: fmt.Sprintf("unresolved reference ${.env.%s} — declare it in the project env: block", r.Name), Severity: Error})
				}
			case "outputs":
				name := r.Name
				if defined[name] {
					continue
				}
				// inside a parallel branch: a capture bound only by a sibling
				// branch has no ordering relative to this one.
				if siblings != nil && boundInSibling(siblings, selfIdx, name) {
					v.add(Finding{Step: st.Run, Rule: 12,
						Msg: fmt.Sprintf("${.outputs.%s} is bound in a sibling parallel branch — concurrent branches have no ordering; reference it after the block", name), Severity: Error})
					continue
				}
				if binder, later := v.laterBound[name]; later {
					rule, msg := 10, fmt.Sprintf("${.outputs.%s} is referenced before %s binds it", name, binder)
					if binder == "stop_background" || v.bgRunning[name] {
						rule = 6
						msg = fmt.Sprintf("${.outputs.%s} is a background capture — it settles only at stop_background; reference it after", name)
					}
					v.add(Finding{Step: st.Run, Rule: rule, Msg: msg, Severity: Error})
					continue
				}
				v.add(Finding{Step: st.Run, Rule: 10,
					Msg: fmt.Sprintf("unresolved reference ${.outputs.%s} — no earlier capture by that name", name), Severity: Error})
			default:
				v.add(Finding{Step: st.Run, Rule: 10,
					Msg: fmt.Sprintf("unresolved reference ${.%s...} — references must be namespaced (.vars/.outputs/.env)", r.Namespace), Severity: Error})
			}
		}
	}

	// track state for later rules
	if st.Run == "background" {
		if m, ok := raw.(map[string]any); ok {
			if n, _ := m["name"].(string); n != "" {
				v.bgRunning[n] = true
			}
		}
	}
	if st.Run == "sample" {
		v.observesLatency = true
	}
	for _, ref := range refsOf(st) {
		if strings.Contains(ref, "meta.durationMs") {
			v.observesLatency = true
		}
	}
	if strings.HasPrefix(section, "method") {
		// An outage-class fault (work can be lost) is what makes a scenario
		// recovery-shaped — declared by the verb (or the step, for faults
		// injected via exec.run/http.post), not matched against a name list.
		effect := res.Spec.Effect
		if st.Effect != "" {
			effect = sdk.Effect(st.Effect)
		}
		if effect == sdk.EffectOutage {
			v.methodHasOutage = true
		}
		if effect == sdk.EffectDegradation {
			v.methodHasDegradation = true
		}
		if kind == sdk.KindAction && (st.As != "" || len(st.Capture) > 0) {
			v.methodCapturesID = true
		}
	}
	if section == "verify" {
		if len(refsOf(st)) > 0 {
			v.verifyAwaitsCapture = true
		}
		if st.Finding != "" {
			v.verifyHasFinding = true
		}
		if st.Run == "assert" {
			if m, ok := raw.(map[string]any); ok {
				if eq, has := m["equals"]; has && fmt.Sprintf("%v", eq) == "1" {
					v.verifyHasExactlyOnce = true
				}
			}
		}
	}

	for name := range bindings(st) {
		defined[name] = true
	}
}

// checkParallel validates a parallel step: branch structure (rule 12), then
// each branch recursively against a branch-local scope (pre-block captures plus
// that branch's own earlier captures), so siblings stay isolated. After the
// block, every branch's captures become visible to following steps.
func (v *scenarioValidator) checkParallel(st *model.Step, section string, defined map[string]bool) {
	branches := decodeBranches(st)
	if len(branches) == 0 {
		v.add(Finding{Step: "parallel", Rule: 12, Msg: "parallel: branches must be a non-empty list", Severity: Error})
		return
	}
	siblings := make([]map[string]bool, len(branches))
	for bi := range branches {
		siblings[bi] = branchBindings(branches[bi])
	}
	for bi := range branches {
		if len(branches[bi]) == 0 {
			v.add(Finding{Step: "parallel", Rule: 12, Msg: fmt.Sprintf("parallel: branch %d is empty", bi), Severity: Error})
			continue
		}
		branchScope := make(map[string]bool, len(defined))
		for k := range defined {
			branchScope[k] = true
		}
		for si := range branches[bi] {
			v.checkStep(&branches[bi][si], section, branchScope, siblings, bi)
		}
	}
	// branch captures become visible to steps after the block.
	for _, bset := range siblings {
		for name := range bset {
			defined[name] = true
		}
	}
}

// checkRepeat validates a repeat step: times >= 1, a non-empty do: body, no
// finding: inside the body (deferred: repeat findings have no aggregate
// semantics yet), and that any background started in the body is also stopped
// in it (else it collides across iterations). The body is then validated in
// order against a body-local scope seeded from the pre-block captures.
func (v *scenarioValidator) checkRepeat(st *model.Step, section string, defined map[string]bool) {
	var w struct {
		Times int          `yaml:"times"`
		Do    []model.Step `yaml:"do"`
	}
	_ = st.With.Decode(&w)
	if w.Times < 1 {
		v.add(Finding{Step: "repeat", Rule: 13, Msg: "repeat: times must be >= 1", Severity: Error})
	}
	if len(w.Do) == 0 {
		v.add(Finding{Step: "repeat", Rule: 13, Msg: "repeat: do must be a non-empty list", Severity: Error})
		return
	}

	started, stopped := map[string]bool{}, map[string]bool{}
	for i := range w.Do {
		bs := &w.Do[i]
		if bs.Finding != "" {
			v.add(Finding{Step: bs.Run, Rule: 13, Severity: Error,
				Msg: "finding: is not allowed inside repeat (no aggregate semantics across iterations yet)"})
		}
		if bs.Run == "background" {
			if m, ok := rawWith(bs).(map[string]any); ok {
				if n, _ := m["name"].(string); n != "" {
					started[n] = true
				}
			}
		}
		if bs.Run == "stop_background" {
			if n := stopName(bs); n != "" {
				stopped[n] = true
			}
		}
	}
	for n := range started {
		if !stopped[n] {
			v.add(Finding{Step: "background", Rule: 13, Severity: Error,
				Msg: fmt.Sprintf("background %q inside repeat must be paired with stop_background in the same body (else it collides each iteration)", n)})
		}
	}

	bodyScope := make(map[string]bool, len(defined))
	for k := range defined {
		bodyScope[k] = true
	}
	for i := range w.Do {
		v.checkStep(&w.Do[i], section, bodyScope, nil, i)
	}
	for k := range bodyScope {
		defined[k] = true // body captures become visible after the block
	}
}

// stopName extracts the target name of a stop_background step, whether given as
// a scalar (with: gen) or a map (with: { name: gen }).
func stopName(st *model.Step) string {
	switch w := rawWith(st).(type) {
	case string:
		return w
	case map[string]any:
		n, _ := w["name"].(string)
		return n
	}
	return ""
}

func boundInSibling(siblings []map[string]bool, selfIdx int, name string) bool {
	for bj := range siblings {
		if bj != selfIdx && siblings[bj][name] {
			return true
		}
	}
	return false
}

// validateComposedDef checks a kind: Provider body: every ${ref} in a verb
// body must reference a param, an earlier body capture, or a declared .env
// value. Composed verbs declare their per-call inputs as params rather than
// reaching into caller vars, but .env is ambient project config (tenant,
// credentials) the engine already passes into the macro scope, so a composed
// REST verb can read it directly instead of threading creds through params.
func validateComposedDef(def *model.ProviderDef, envSet map[string]bool) []Finding {
	var out []Finding
	for verb, cv := range def.Verbs {
		names, _ := cv.ParamNames()
		paramSet := map[string]bool{}
		for _, n := range names {
			paramSet[n] = true
		}
		outputs := map[string]bool{} // body captures, grown in order
		steps := cv.Do
		if cv.Probe != nil {
			steps = []model.Step{*cv.Probe}
		}
		for i := range steps {
			st := &steps[i]
			for _, ref := range refsOf(st) {
				for _, r := range jqx.NSRefs(ref) {
					switch r.Namespace {
					case "params":
						if !paramSet[r.Name] {
							out = append(out, Finding{File: def.File, Step: st.Run, Rule: 10, Severity: Error,
								Msg: fmt.Sprintf("provider %s verb %s: ${.params.%s} is not a declared param", def.Name, verb, r.Name)})
						}
					case "outputs":
						if !outputs[r.Name] {
							out = append(out, Finding{File: def.File, Step: st.Run, Rule: 10, Severity: Error,
								Msg: fmt.Sprintf("provider %s verb %s: ${.outputs.%s} is not an earlier capture", def.Name, verb, r.Name)})
						}
					case "env":
						if !envSet[r.Name] {
							out = append(out, Finding{File: def.File, Step: st.Run, Rule: 10, Severity: Error,
								Msg: fmt.Sprintf("provider %s verb %s: ${.env.%s} is not declared in the project env: block", def.Name, verb, r.Name)})
						}
					default:
						out = append(out, Finding{File: def.File, Step: st.Run, Rule: 10, Severity: Error,
							Msg: fmt.Sprintf("provider %s verb %s: ${.%s...} — composed verbs reference .params, .env, or an earlier .outputs capture", def.Name, verb, r.Namespace)})
					}
				}
			}
			for name := range bindings(st) {
				outputs[name] = true
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

// branchBindings returns the capture names a branch binds, recursively
// (including bindings inside any nested parallel branches).
func branchBindings(branch []model.Step) map[string]bool {
	out := map[string]bool{}
	collectBindings(branch, func(name, _ string) { out[name] = true })
	return out
}

// collectBindings calls sink(name, verb) for every capture bound by steps,
// recursing into parallel branches so nested bindings are seen too.
func collectBindings(steps []model.Step, sink func(name, verb string)) {
	for i := range steps {
		st := &steps[i]
		for name := range bindings(st) {
			sink(name, st.Run)
		}
		for _, branch := range decodeBranches(st) {
			collectBindings(branch, sink)
		}
	}
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
