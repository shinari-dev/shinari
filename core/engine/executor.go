// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shinari-dev/shinari/core/builtins"
	"github.com/shinari-dev/shinari/core/interp"
	"github.com/shinari-dev/shinari/core/jqx"
	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/core/registry"
	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

// Options are run-level knobs. The CLI maps env (KEEP_UP=1) onto them —
// core never reads the environment itself.
type Options struct {
	KeepUp bool // skip teardown, preserve the stack
	DryRun bool // skip kind: action steps
	Clock  func() time.Time
}

func (o Options) now() time.Time {
	if o.Clock != nil {
		return o.Clock()
	}
	return time.Now()
}

type bgHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
	result sdk.VerbResult
	err    error
}

type runner struct {
	reg      *registry.Registry
	emit     Emitter
	sc       *model.Scenario
	opts     Options
	captures map[string]any
	vars     map[string]any
	bg       map[string]*bgHandle
	res      *ScenarioResult
}

// RunScenario executes one scenario: setup → steadyState (gate) → method
// → steadyState (recovery) → verify → teardown (always).
func RunScenario(ctx context.Context, sc *model.Scenario, projectVars map[string]any, reg *registry.Registry, em Emitter, opts Options) ScenarioResult {
	if em == nil {
		em = EmitterFunc(func(Event) {})
	}
	vars := map[string]any{}
	for k, v := range projectVars {
		vars[k] = v
	}
	for k, v := range sc.Vars {
		vars[k] = v
	}
	r := &runner{
		reg: reg, emit: em, sc: sc, opts: opts,
		captures: map[string]any{},
		vars:     vars,
		bg:       map[string]*bgHandle{},
		res: &ScenarioResult{
			Name: sc.Name, Description: sc.Description, Suite: sc.Suite,
			Start: opts.now(),
		},
	}
	em.Emit(Event{Type: EvScenarioStarted, Time: opts.now(), Scenario: sc.Name})

	verdict, reason := r.timeline(ctx)

	r.teardown(ctx)
	r.res.Verdict = verdict
	r.res.Reason = reason
	r.res.End = opts.now()
	em.Emit(Event{Type: EvScenarioFinished, Time: opts.now(), Scenario: sc.Name,
		Payload: map[string]any{"verdict": string(verdict), "reason": reason}})
	return *r.res
}

// timeline runs everything except teardown and resolves the verdict.
func (r *runner) timeline(ctx context.Context) (ScenarioVerdict, string) {
	if msg, ok := r.runSection(ctx, "setup", "", r.sc.Setup, true); !ok {
		return ScenarioErrored, "setup failed: " + msg
	}
	if len(r.sc.SteadyState) > 0 {
		if msg, ok := r.runSection(ctx, "steadyState", "", r.sc.SteadyState, true); !ok {
			return ScenarioInconclusive, "steadyState failed before method — never healthy: " + msg
		}
	}
	for _, ph := range r.sc.Method {
		r.emit.Emit(Event{Type: EvPhaseStarted, Time: r.opts.now(), Scenario: r.sc.Name, Phase: ph.Phase})
		if msg, ok := r.runSection(ctx, "method", ph.Phase, ph.Steps, true); !ok {
			return ScenarioFailed, fmt.Sprintf("method phase %q failed: %s", ph.Phase, msg)
		}
	}
	var failures []string
	if len(r.sc.SteadyState) > 0 {
		if msg, ok := r.runSection(ctx, "steadyState:recovery", "", r.sc.SteadyState, false); !ok {
			failures = append(failures, "steadyState did not recover after method: "+msg)
		}
	}
	if msg, ok := r.runSection(ctx, "verify", "", r.sc.Verify, false); !ok {
		failures = append(failures, msg)
	}
	if len(failures) > 0 {
		return ScenarioFailed, strings.Join(failures, "; ")
	}
	return ScenarioPassed, ""
}

func (r *runner) teardown(ctx context.Context) {
	if r.opts.KeepUp {
		return
	}
	steps := r.sc.Teardown
	if !r.sc.HasTeardown {
		// Default teardown: <lifecycle>.down (challenge #3: an explicit
		// teardown replaces this entirely).
		if lc := r.reg.Lifecycle(); len(lc) == 1 {
			steps = []model.Step{{Run: lc[0] + ".down"}}
		}
	}
	// Teardown always runs and never changes the verdict.
	r.runSection(context.WithoutCancel(ctx), "teardown", "", steps, false)
}

// runSection executes steps. stopOnFail sections (setup/steady-gate/method)
// stop at the first failure; cumulative sections (verify/teardown) run all
// steps and aggregate. ok=false when a non-finding check failed.
func (r *runner) runSection(ctx context.Context, section, phase string, steps []model.Step, stopOnFail bool) (string, bool) {
	var failures []string
	for i := range steps {
		st := &steps[i]
		sr := r.runStep(ctx, section, phase, st)
		r.res.Steps = append(r.res.Steps, sr)
		switch sr.Verdict {
		case CheckFail:
			msg := sr.Err
			if sr.Desc != "" {
				msg = sr.Desc + ": " + msg
			}
			failures = append(failures, msg)
			if stopOnFail {
				return msg, false
			}
		case CheckPass:
			if isAssertionLike(r.reg, st) && section != "teardown" {
				label := stepLabel(st)
				if section == "verify" || strings.HasPrefix(section, "steadyState") {
					r.res.Held = append(r.res.Held, label)
				}
			}
		}
	}
	return strings.Join(failures, "; "), len(failures) == 0
}

func isAssertionLike(reg *registry.Registry, st *model.Step) bool {
	res, err := reg.Resolve(st.Run)
	if err != nil {
		return false
	}
	kind := res.Spec.Kind
	if st.Kind != "" {
		kind = sdk.Kind(st.Kind)
	}
	return kind == sdk.KindAssertion
}

// runStep executes one step envelope: resolve → interpolate → bind →
// dispatch → read/as/capture → verdict (incl. finding logic).
func (r *runner) runStep(ctx context.Context, section, phase string, st *model.Step) StepResult {
	sr := StepResult{Section: section, Phase: phase, Run: st.Run, Desc: st.Desc, Start: r.opts.now()}
	r.emit.Emit(Event{Type: EvStepStarted, Time: sr.Start, Scenario: r.sc.Name,
		Section: section, Phase: phase, Step: stepLabel(st), Verb: st.Run})

	finish := func(v CheckVerdict, errMsg string) StepResult {
		sr.Verdict = v
		sr.Err = errMsg
		sr.End = r.opts.now()
		r.emit.Emit(Event{Type: stepEventType[v], Time: sr.End, Scenario: r.sc.Name,
			Section: section, Phase: phase, Step: stepLabel(st), Verb: st.Run,
			Payload: map[string]any{"verdict": string(v), "error": errMsg}})
		return sr
	}
	skipOrFail := func(err error) StepResult {
		if st.OnAbsent == "skip" {
			sr.SkipReason = st.SkipReason
			if sr.SkipReason == "" {
				sr.SkipReason = err.Error()
			}
			return finish(CheckSkip, "")
		}
		return r.judge(st, finish, err)
	}

	res, err := r.reg.Resolve(st.Run)
	if err != nil {
		return skipOrFail(err)
	}
	kind := res.Spec.Kind
	if st.Kind != "" {
		kind = sdk.Kind(st.Kind) // the exec.run override
	}
	if r.opts.DryRun && kind == sdk.KindAction {
		sr.SkipReason = "dry-run skips actions"
		return finish(CheckSkip, "")
	}

	withVal, err := r.decodeWith(st, r.scope())
	if err != nil {
		return skipOrFail(err)
	}

	runCtx := ctx
	if st.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(st.Timeout*float64(time.Second)))
		defer cancel()
	}
	result, err := r.execVerb(runCtx, res, st.Run, withVal, r.scope())
	if err != nil {
		return r.judge(st, finish, err)
	}

	if _, err = applyBindings(st, result, func(name string, v any) {
		r.captures[name] = v
		if sr.Captured == nil {
			sr.Captured = map[string]any{}
		}
		sr.Captured[name] = v
	}); err != nil {
		return r.judge(st, finish, err)
	}

	effect := res.Spec.Effect
	if st.Effect != "" {
		effect = sdk.Effect(st.Effect) // a fault injected via a polymorphic verb (exec.run, http.post)
	}
	if effect != sdk.EffectNone && section == "method" {
		r.emit.Emit(Event{Type: EvFaultInjected, Time: r.opts.now(), Scenario: r.sc.Name,
			Section: section, Phase: phase, Step: stepLabel(st), Verb: st.Run})
		r.res.Injected = append(r.res.Injected, stepLabel(st))
	}

	return r.judge(st, finish, nil)
}

// judge applies the findings ledger to a step outcome: a failing
// finding: is FINDING (and keeps the scenario green); a passing finding:
// is a FAIL that says "promote me".
func (r *runner) judge(st *model.Step, finish func(CheckVerdict, string) StepResult, err error) StepResult {
	if st.Finding == "" {
		if err != nil {
			return finish(CheckFail, err.Error())
		}
		return finish(CheckPass, "")
	}
	if err != nil {
		r.res.Findings = append(r.res.Findings, FindingRecord{
			Scenario: r.sc.Name, Narrative: st.Finding, Check: stepLabel(st), Detail: err.Error(),
		})
		r.emit.Emit(Event{Type: EvFindingRecorded, Time: r.opts.now(), Scenario: r.sc.Name,
			Step: stepLabel(st), Verb: st.Run,
			Payload: map[string]any{"narrative": st.Finding, "detail": err.Error()}})
		sr := finish(CheckFinding, "")
		return sr
	}
	r.res.Findings = append(r.res.Findings, FindingRecord{
		Scenario: r.sc.Name, Narrative: st.Finding, Check: stepLabel(st), NowPasses: true,
	})
	r.emit.Emit(Event{Type: EvFindingRecorded, Time: r.opts.now(), Scenario: r.sc.Name,
		Step: stepLabel(st), Verb: st.Run,
		Payload: map[string]any{"narrative": st.Finding, "nowPasses": true}})
	return finish(CheckFail, fmt.Sprintf("finding now passes — the gap %q was fixed; promote this to a hard assertion", st.Finding))
}

func (r *runner) scope() interp.Scope {
	return interp.Scope{Vars: r.vars, Captures: r.captures}
}

func (r *runner) decodeWith(st *model.Step, scope interp.Scope) (any, error) {
	raw, err := st.DecodeWith()
	if err != nil {
		return nil, fmt.Errorf("step %s: bad with: %w", st.Run, err)
	}
	v, err := scope.Any(raw)
	if err != nil {
		return nil, fmt.Errorf("step %s: %w", st.Run, err)
	}
	return v, nil
}

// applyBindings evaluates a step's read: transform, then binds its as:/
// capture: outputs through sink. The top-level executor and composed-macro
// expansion share this; they differ only in where bindings land (the run
// captures vs. a macro-local scope), which the sink abstracts.
func applyBindings(st *model.Step, result sdk.VerbResult, sink func(name string, v any)) (any, error) {
	value := result.Value
	if st.Read != "" {
		v, err := jqx.Eval(st.Read, value)
		if err != nil {
			return nil, err
		}
		value = v
	}
	if st.As != "" {
		sink(st.As, envelope(result.Output, value, result.Meta))
	}
	for name, expr := range st.Capture {
		v, err := jqx.Eval(expr, value)
		if err != nil {
			return nil, err
		}
		sink(name, v)
	}
	return value, nil
}

// envelope wraps a verb result for `as:`: the payload, its raw output, and
// metadata (durationMs plus provider facts). read:/capture: still operate on
// the payload; only `as:` binds this whole shape.
func envelope(output string, value any, meta map[string]any) map[string]any {
	if meta == nil {
		meta = map[string]any{}
	}
	return map[string]any{"value": value, "output": output, "meta": meta}
}

// execVerb dispatches a resolved verb: language builtin, composed macro,
// or native provider.
func (r *runner) execVerb(ctx context.Context, res registry.Resolution, run string, withVal any, scope interp.Scope) (sdk.VerbResult, error) {
	args, err := registry.BindArgs(res.Spec, withVal)
	if err != nil {
		return sdk.VerbResult{}, err
	}
	start := r.opts.now()
	var result sdk.VerbResult
	switch {
	case res.Builtin != "":
		result, err = r.execBuiltin(ctx, res.Builtin, args, scope)
	case res.Composed != nil:
		result, err = r.execComposed(ctx, run, res, args)
	default:
		result, err = res.Instance.Native.Run(ctx, registry.VerbName(run), args)
	}
	if result.Meta == nil {
		result.Meta = map[string]any{}
	}
	result.Meta["durationMs"] = float64(r.opts.now().Sub(start).Milliseconds())
	return result, err
}

// execComposed expands a macro: body steps run with a macro-local scope
// (params as captures, caller vars); the result is the last step's value.
func (r *runner) execComposed(ctx context.Context, run string, res registry.Resolution, args map[string]any) (sdk.VerbResult, error) {
	names, _ := res.Composed.ParamNames()
	local := map[string]any{}
	for _, n := range names {
		local[n] = args[n] // missing optional params bind to nil → ""
	}
	scope := interp.Scope{Vars: r.vars, Captures: local}

	steps := res.Composed.Do
	if res.Composed.Probe != nil {
		steps = []model.Step{*res.Composed.Probe}
	}
	var last sdk.VerbResult
	for i := range steps {
		st := &steps[i]
		leafRes, err := r.reg.Resolve(st.Run)
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("%s: %w", run, err)
		}
		raw, err := st.DecodeWith()
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("%s: %w", run, err)
		}
		withVal, err := scope.Any(raw)
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("%s: %w", run, err)
		}
		result, err := r.execVerb(ctx, leafRes, st.Run, withVal, scope)
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("%s → %s: %w", run, st.Run, err)
		}
		value, err := applyBindings(st, result, func(name string, v any) { local[name] = v })
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("%s → %s: %w", run, st.Run, err)
		}
		result.Value = value
		last = result
	}
	return last, nil
}

func (r *runner) execBuiltin(ctx context.Context, name string, args map[string]any, scope interp.Scope) (sdk.VerbResult, error) {
	switch name {
	case "assert":
		op, operand, err := builtins.ExtractOperator(args)
		if err != nil {
			return sdk.VerbResult{}, err
		}
		pass, msg, err := builtins.Check(args["of"], op, operand)
		if err != nil {
			return sdk.VerbResult{}, err
		}
		if !pass {
			return sdk.VerbResult{}, fmt.Errorf("assert failed: %s", msg)
		}
		return sdk.VerbResult{Value: true}, nil

	case "sleep":
		secs, ok := conv.ToFloat(args["seconds"])
		if !ok {
			return sdk.VerbResult{}, fmt.Errorf("sleep: seconds must be a number, got %v", args["seconds"])
		}
		return sdk.VerbResult{}, sleepCtx(ctx, secs)

	case "wait_until":
		return r.execWaitUntil(ctx, args, scope)

	case "background":
		bgName, _ := args["name"].(string)
		stepMap, _ := args["step"].(map[string]any)
		if bgName == "" || stepMap == nil {
			return sdk.VerbResult{}, fmt.Errorf("background needs { name, step }")
		}
		bgCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		h := &bgHandle{cancel: cancel, done: make(chan struct{})}
		r.bg[bgName] = h
		go func() {
			defer close(h.done)
			h.result, h.err = r.execStepMap(bgCtx, stepMap, scope)
		}()
		return sdk.VerbResult{}, nil

	case "stop_background":
		bgName, _ := args["name"].(string)
		h, ok := r.bg[bgName]
		if !ok {
			return sdk.VerbResult{}, fmt.Errorf("stop_background: no background task named %q", bgName)
		}
		h.cancel()
		<-h.done
		delete(r.bg, bgName)
		if h.err != nil {
			// A canceled load generator is the expected shape; surface its
			// output, not a failure.
			return sdk.VerbResult{Value: h.result.Output, Output: h.result.Output}, nil
		}
		return h.result, nil

	case "sample":
		return r.execSample(ctx, args, scope)
	}
	return sdk.VerbResult{}, fmt.Errorf("unknown builtin %q", name)
}

// sleepCtx waits for seconds, or returns ctx.Err() if the context is cancelled
// first. Shared by sleep and sample's inter-sample interval.
func sleepCtx(ctx context.Context, seconds float64) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(seconds * float64(time.Second))):
		return nil
	}
}

// execSample runs a probe repeatedly (count times or for duration seconds, at
// interval) and aggregates the results into an Observation whose value is
// { n, errors, errorRate, min, max, mean, p50, p95, p99 }. Latencies are in ms.
func (r *runner) execSample(ctx context.Context, args map[string]any, scope interp.Scope) (sdk.VerbResult, error) {
	probeMap, _ := args["probe"].(map[string]any)
	if probeMap == nil {
		return sdk.VerbResult{}, fmt.Errorf("sample needs a probe: step")
	}
	count := 0
	if n, ok := conv.ToFloat(args["count"]); ok {
		count = int(n)
	}
	duration := 0.0
	if d, ok := conv.ToFloat(args["duration"]); ok {
		duration = d
	}
	if count <= 0 && duration <= 0 {
		return sdk.VerbResult{}, fmt.Errorf("sample needs count: or duration:")
	}
	interval := 0.0
	if v, ok := conv.ToFloat(args["interval"]); ok {
		interval = v
	}
	deadline := r.opts.now().Add(time.Duration(duration * float64(time.Second)))

	var lats []float64
	n, errs := 0, 0
	for {
		if count > 0 && n >= count {
			break
		}
		if duration > 0 && !r.opts.now().Before(deadline) {
			break
		}
		start := r.opts.now()
		_, perr := r.execStepMap(ctx, probeMap, scope)
		lats = append(lats, float64(r.opts.now().Sub(start).Milliseconds()))
		n++
		if perr != nil {
			errs++
		}
		if err := sleepCtx(ctx, interval); err != nil {
			return sdk.VerbResult{}, err
		}
	}
	sort.Float64s(lats)
	return sdk.VerbResult{Value: map[string]any{
		"n":         float64(n),
		"errors":    float64(errs),
		"errorRate": ratio(errs, n),
		"min":       at(lats, 0),
		"max":       at(lats, len(lats)-1),
		"mean":      mean(lats),
		"p50":       percentile(lats, 50),
		"p95":       percentile(lats, 95),
		"p99":       percentile(lats, 99),
	}}, nil
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func at(xs []float64, i int) float64 {
	if i < 0 || i >= len(xs) {
		return 0
	}
	return xs[i]
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// percentile is the nearest-rank value of sorted xs at the p-th percentile.
func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p*len(sorted)+99)/100 - 1 // ceil(p/100 * n) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// execWaitUntil blocks the timeline on an observed event: poll the
// probe until the condition holds or timeout. The gate is the engine; the
// observation is a provider probe.
func (r *runner) execWaitUntil(ctx context.Context, args map[string]any, scope interp.Scope) (sdk.VerbResult, error) {
	probeMap, _ := args["probe"].(map[string]any)
	if probeMap == nil {
		return sdk.VerbResult{}, fmt.Errorf("wait_until needs a probe: step")
	}
	op, operand, err := builtins.ExtractOperator(args)
	if err != nil {
		return sdk.VerbResult{}, fmt.Errorf("wait_until: %w", err)
	}
	timeout, ok := conv.ToFloat(args["timeout"])
	if !ok {
		return sdk.VerbResult{}, fmt.Errorf("wait_until: timeout must be a number of seconds, got %v", args["timeout"])
	}
	interval := 1.0
	if v, ok := conv.ToFloat(args["interval"]); ok {
		interval = v
	}
	readExpr, _ := args["read"].(string)

	deadline := time.After(time.Duration(timeout * float64(time.Second)))
	var lastObserved any
	for {
		result, perr := r.execStepMap(ctx, probeMap, scope)
		if perr == nil {
			value := result.Value
			if readExpr != "" {
				value, perr = jqx.Eval(readExpr, value)
			}
			if perr == nil {
				lastObserved = value
				pass, _, cerr := builtins.Check(value, op, operand)
				if cerr != nil {
					return sdk.VerbResult{}, fmt.Errorf("wait_until: %w", cerr)
				}
				if pass {
					r.emit.Emit(Event{Type: EvGateObserved, Time: r.opts.now(), Scenario: r.sc.Name,
						Verb: "wait_until", Payload: map[string]any{"observed": value, "op": op}})
					return sdk.VerbResult{Value: value}, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return sdk.VerbResult{}, ctx.Err()
		case <-deadline:
			return sdk.VerbResult{}, fmt.Errorf("wait_until: condition (%s %v) not observed within %gs; last observed: %v", op, operand, timeout, lastObserved)
		case <-time.After(time.Duration(interval * float64(time.Second))):
		}
	}
}

// execStepMap runs a nested step given as plain data (wait_until probe,
// background step): { run, with, read }.
func (r *runner) execStepMap(ctx context.Context, m map[string]any, scope interp.Scope) (sdk.VerbResult, error) {
	run, _ := m["run"].(string)
	if run == "" {
		return sdk.VerbResult{}, fmt.Errorf("nested step needs run:")
	}
	res, err := r.reg.Resolve(run)
	if err != nil {
		return sdk.VerbResult{}, err
	}
	withVal, err := scope.Any(m["with"])
	if err != nil {
		return sdk.VerbResult{}, err
	}
	result, err := r.execVerb(ctx, res, run, withVal, scope)
	if err != nil {
		return result, err
	}
	if readExpr, _ := m["read"].(string); readExpr != "" {
		v, rerr := jqx.Eval(readExpr, result.Value)
		if rerr != nil {
			return result, rerr
		}
		result.Value = v
	}
	return result, nil
}

func stepLabel(st *model.Step) string {
	if st.Desc != "" {
		return st.Desc
	}
	return st.Run
}
