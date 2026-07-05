// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/interp"
	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/core/registry"
	"github.com/shinari-dev/shinari/sdk"
)

// fakeSUT is a scriptable system under test: a lifecycle provider plus a
// scriptable probe whose successive values are programmed per test.
type fakeSUT struct {
	mu         sync.Mutex
	calls      []string
	script     map[string][]any // verb -> successive Values (last repeats)
	fails      map[string]error // verb -> permanent failure
	blockExits atomic.Int32     // bumped when a block verb returns after cancellation
	closes     atomic.Int32     // bumped when Close is called
}

func (f *fakeSUT) Type() string                   { return "fakesut" }
func (f *fakeSUT) Configure(map[string]any) error { return nil }
func (f *fakeSUT) Verbs() []sdk.VerbSpec {
	return []sdk.VerbSpec{
		{Name: "up", Kind: sdk.KindAction, SideEffects: true, Primary: "services"},
		{Name: "down", Kind: sdk.KindAction, SideEffects: true},
		{Name: "kill", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "service"},
		{Name: "submit", Kind: sdk.KindAction, SideEffects: true, Primary: "job"},
		{Name: "status", Kind: sdk.KindProbe, Primary: "of"},
		{Name: "count", Kind: sdk.KindProbe, Primary: "job"},
		{Name: "echo", Kind: sdk.KindProbe, Primary: "of"},
		{Name: "smoke", Kind: sdk.KindAssertion},
		{Name: "block", Kind: sdk.KindProbe},
	}
}
func (f *fakeSUT) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	// block models a long-running background task: it runs until its context is
	// cancelled, then records that it exited. Handled before the mutex so it
	// never holds the lock while parked.
	if verb == "block" {
		<-ctx.Done()
		f.blockExits.Add(1)
		return sdk.VerbResult{}, ctx.Err()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, verb)
	if err, ok := f.fails[verb]; ok && err != nil {
		return sdk.VerbResult{}, err
	}
	if verb == "echo" {
		return sdk.VerbResult{Value: args["of"]}, nil
	}
	if vals, ok := f.script[verb]; ok && len(vals) > 0 {
		v := vals[0]
		if len(vals) > 1 {
			f.script[verb] = vals[1:]
		}
		return sdk.VerbResult{Value: v}, nil
	}
	return sdk.VerbResult{Value: "ok"}, nil
}

// closes counts Close calls so a test can assert the engine releases providers.
func (f *fakeSUT) Close() error { f.closes.Add(1); return nil }

func (f *fakeSUT) callCount(verb string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == verb {
			n++
		}
	}
	return n
}

// currentSUT is the provider the "fakesut" type resolves to. Registration
// happens once (sdk.Register panics on a duplicate); tests vary behavior by
// setting this before building the registry, which constructs the instance.
var currentSUT sdk.Provider

func init() { sdk.Register("fakesut", func() sdk.Provider { return currentSUT }) }

func newWorld(t *testing.T, scenarioYAML string) (*fakeSUT, *model.Scenario, *registry.Registry) {
	t.Helper()
	sut := &fakeSUT{script: map[string][]any{}, fails: map[string]error{}}
	currentSUT = sut

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "project.yml"),
		[]byte("apiVersion: shinari/v1\nkind: Project\nname: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := discover.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := registry.New(set, map[string]model.ProviderConfig{
		"sut": {Source: "fakesut"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := model.ParseScenario([]byte(scenarioYAML), "s.yml")
	if err != nil {
		t.Fatal(err)
	}
	return sut, sc, reg
}

func run(t *testing.T, sut *fakeSUT, sc *model.Scenario, reg *registry.Registry) (ScenarioResult, *Recorder) {
	t.Helper()
	rec := &Recorder{}
	res := RunScenario(context.Background(), sc, nil, reg, rec, Options{})
	return res, rec
}

const passingScenario = `
apiVersion: shinari/v1
kind: Scenario
name: happy
vars: { n: 1 }
setup:
  - { run: sut.up, with: [app] }
steadyState:
  - { run: sut.smoke }
method:
  - phase: "inject"
    steps:
      - { run: sut.submit, with: sleep, as: job }
      - { run: sut.kill, with: app }
verify:
  - { run: sut.count, with: sleep, as: total }
  - { run: assert, with: { of: "${.outputs.total.value}", equals: "${.vars.n}" }, desc: "exactly once" }
`

func TestPassedScenario(t *testing.T) {
	sut, sc, reg := newWorld(t, passingScenario)
	sut.script["count"] = []any{1}
	res, rec := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	// default teardown ran (sut is the lifecycle provider)
	if sut.callCount("down") != 1 {
		t.Errorf("default teardown must call down once, got %d", sut.callCount("down"))
	}
	// steadyState ran twice: gate + recovery
	if sut.callCount("smoke") != 2 {
		t.Errorf("steadyState must run before AND after method, got %d", sut.callCount("smoke"))
	}
	// fault.injected emitted for kill in method
	found := false
	for _, e := range rec.Events {
		if e.Type == EvFaultInjected && e.Verb == "sut.kill" {
			found = true
		}
	}
	if !found {
		t.Error("missing fault.injected event for sut.kill")
	}
	if len(res.Held) == 0 || res.Held[0] != "sut.smoke" {
		t.Errorf("held = %v", res.Held)
	}
}

func TestRunScenarioInjectsEnv(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: env-read
verify:
  - { run: assert, with: { of: "${.env.REGION}", equals: "us-east-1" } }
`
	_, sc, reg := newWorld(t, src)
	res := RunScenario(context.Background(), sc, nil, reg, &Recorder{},
		Options{Env: map[string]any{"REGION": "us-east-1"}})
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s, reason = %s", res.Verdict, res.Reason)
	}
}

func TestAsBindsEnvelopeWithDuration(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: envelope
verify:
  - { run: sut.count, as: rsp }
  - { run: assert, with: { of: "${.outputs.rsp.value}", equals: "ok" } }
  - { run: assert, with: { of: "${.outputs.rsp.meta.durationMs}", gte: 0 } }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
}

func TestStepTimeoutFails(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: timeout
verify:
  - { run: sleep, with: { seconds: 5 }, timeout: 0.1 }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("verdict = %s, want FAILED (timeout)", res.Verdict)
	}
}

func TestScenarioTimeoutFails(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: scenariotimeout
timeout: 0.1
verify:
  - { run: sleep, with: { seconds: 5 } }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("verdict = %s, want FAILED", res.Verdict)
	}
	if !strings.Contains(res.Reason, "scenario exceeded timeout") {
		t.Fatalf("reason = %q, want it to name the scenario timeout", res.Reason)
	}
	// teardown still runs after a scenario timeout
	if sut.callCount("down") != 1 {
		t.Errorf("teardown must run after timeout, down called %d times", sut.callCount("down"))
	}
}

func TestTimedOutStepIsMarked(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: timedoutmark
verify:
  - { run: sleep, with: { seconds: 5 }, timeout: 0.1 }
`
	sut, sc, reg := newWorld(t, src)
	res, rec := run(t, sut, sc, reg)

	var sleepStep *StepResult
	for i := range res.Steps {
		if res.Steps[i].Run == "sleep" {
			sleepStep = &res.Steps[i]
		}
	}
	if sleepStep == nil {
		t.Fatal("no sleep step in result")
	}
	if !sleepStep.TimedOut {
		t.Errorf("sleep step should be marked TimedOut")
	}

	// the event payload carries it too, so Reduce can rebuild it
	found := false
	for _, e := range rec.Events {
		if e.Type == EvStepFailed && e.Verb == "sleep" {
			if to, _ := e.Payload["timedOut"].(bool); to {
				found = true
			}
		}
	}
	if !found {
		t.Error("step.failed event for sleep must carry timedOut:true")
	}
}

func TestSampleAggregates(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: sample
verify:
  - { run: sample, with: { probe: { run: sut.count }, count: 5 }, as: load }
  - { run: assert, with: { of: "${.outputs.load.value.n}", equals: 5 } }
  - { run: assert, with: { of: "${.outputs.load.value.errorRate}", equals: 0 } }
  - { run: assert, with: { of: "${.outputs.load.value.p99}", gte: 0 } }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
}

func TestSampleCountsErrors(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: sample-errors
verify:
  - { run: sample, with: { probe: { run: sut.count }, count: 4 }, as: load }
  - { run: assert, with: { of: "${.outputs.load.value.errors}", equals: 4 } }
  - { run: assert, with: { of: "${.outputs.load.value.errorRate}", equals: 1 } }
`
	sut, sc, reg := newWorld(t, src)
	sut.fails["count"] = fmt.Errorf("boom") // every probe call errors
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
}

func TestSampleByDuration(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: sample-duration
verify:
  - { run: sample, with: { probe: { run: sut.count }, duration: 0.2, interval: 0.05 }, as: load }
  - { run: assert, with: { of: "${.outputs.load.value.n}", gte: 1 } }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
}

func TestFailedOnVerifyRegression(t *testing.T) {
	sut, sc, reg := newWorld(t, passingScenario)
	sut.script["count"] = []any{2} // duplicate: regression
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("verdict = %s", res.Verdict)
	}
	if !strings.Contains(res.Reason, "exactly once") {
		t.Errorf("reason should name the failed check: %s", res.Reason)
	}
	if sut.callCount("down") != 1 {
		t.Error("teardown must still run on failure")
	}
}

func TestErroredOnSetupFailure(t *testing.T) {
	sut, sc, reg := newWorld(t, passingScenario)
	sut.fails["up"] = fmt.Errorf("compose exploded")
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioErrored {
		t.Fatalf("verdict = %s", res.Verdict)
	}
	if sut.callCount("submit") != 0 {
		t.Error("method must not run after setup failure")
	}
	if sut.callCount("down") != 1 {
		t.Error("teardown must run after setup failure")
	}
}

func TestInconclusiveOnSteadyStateGate(t *testing.T) {
	sut, sc, reg := newWorld(t, passingScenario)
	sut.fails["smoke"] = fmt.Errorf("system not healthy")
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioInconclusive {
		t.Fatalf("verdict = %s", res.Verdict)
	}
	if sut.callCount("submit") != 0 {
		t.Error("method must not run when never healthy")
	}
}

func TestFailedOnSteadyStateRecovery(t *testing.T) {
	sut, sc, _ := newWorld(t, passingScenario)
	// smoke passes the gate once, then fails on the recovery re-run
	sutWrap := &flipFail{inner: sut, failAfter: 1, verb: "smoke"}
	currentSUT = sutWrap
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "project.yml"), []byte("apiVersion: shinari/v1\nkind: Project\nname: t\n"), 0o644)
	set, _ := discover.Load(dir)
	reg2, err := registry.New(set, map[string]model.ProviderConfig{"sut": {Source: "fakesut"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sut.script["count"] = []any{1}
	res, _ := run(t, sut, sc, reg2)
	if res.Verdict != ScenarioFailed || !strings.Contains(res.Reason, "recover") {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
}

// flipFail passes a verb N times then fails it.
type flipFail struct {
	inner     *fakeSUT
	verb      string
	failAfter int
	count     int
}

func (f *flipFail) Type() string                     { return f.inner.Type() }
func (f *flipFail) Configure(c map[string]any) error { return f.inner.Configure(c) }
func (f *flipFail) Verbs() []sdk.VerbSpec            { return f.inner.Verbs() }
func (f *flipFail) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	if verb == f.verb {
		f.count++
		if f.count > f.failAfter {
			return sdk.VerbResult{}, fmt.Errorf("%s degraded after fault", verb)
		}
	}
	return f.inner.Run(ctx, verb, args)
}

const findingScenario = `
apiVersion: shinari/v1
kind: Scenario
name: gap
setup:
  - { run: sut.up, with: [app] }
verify:
  - { run: sut.count, with: sleep, as: total }
  - { run: assert, with: { of: "${.outputs.total.value}", equals: 1 }, desc: "exactly once",
      finding: "duplicates occur after a crash; operator dedupes manually" }
`

func TestFindingKeepsScenarioGreen(t *testing.T) {
	sut, sc, reg := newWorld(t, findingScenario)
	sut.script["count"] = []any{2} // the known gap: duplicates
	res, rec := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("a failing finding must keep the scenario PASSED, got %s (%s)", res.Verdict, res.Reason)
	}
	var step *StepResult
	for i := range res.Steps {
		if res.Steps[i].Finding != "" || res.Steps[i].Verdict == CheckFinding {
			step = &res.Steps[i]
		}
	}
	if step == nil || step.Verdict != CheckFinding {
		t.Fatalf("check verdict must be FINDING: %+v", res.Steps)
	}
	if len(res.Findings) != 1 || res.Findings[0].NowPasses {
		t.Fatalf("findings = %+v", res.Findings)
	}
	found := false
	for _, e := range rec.Events {
		if e.Type == EvFindingRecorded {
			found = true
		}
	}
	if !found {
		t.Error("missing finding.recorded event")
	}
}

func TestFindingThatPassesFailsTheRun(t *testing.T) {
	sut, sc, reg := newWorld(t, findingScenario)
	sut.script["count"] = []any{1} // the gap was fixed!
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("a passing finding must FAIL the scenario (promote it), got %s", res.Verdict)
	}
	if !strings.Contains(res.Reason, "promote") {
		t.Errorf("reason must say promote: %s", res.Reason)
	}
	if len(res.Findings) != 1 || !res.Findings[0].NowPasses {
		t.Fatalf("findings = %+v", res.Findings)
	}
}

func TestWaitUntilGatesOnObservedEvent(t *testing.T) {
	sut, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: gated
method:
  - phase: "wait for RUNNING"
    steps:
      - { run: wait_until, with: { probe: { run: sut.status, with: j1 }, in: [RUNNING], timeout: 5, interval: 0.01 } }
verify:
  - { run: assert, with: { of: 1, equals: 1 } }
`)
	sut.script["status"] = []any{"PENDING", "PENDING", "RUNNING"}
	res, rec := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	if got := sut.callCount("status"); got != 3 {
		t.Errorf("probe should be polled exactly until observed: %d calls", got)
	}
	gated := false
	for _, e := range rec.Events {
		if e.Type == EvGateObserved {
			gated = true
			if e.Payload["observed"] != "RUNNING" {
				t.Errorf("payload = %v", e.Payload)
			}
		}
	}
	if !gated {
		t.Error("missing gate.observed event")
	}
}

func TestWaitUntilTimeoutNamesLastObserved(t *testing.T) {
	sut, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: gated
verify:
  - { run: wait_until, with: { probe: { run: sut.status, with: j1 }, equals: RUNNING, timeout: 0.05, interval: 0.01 } }
`)
	sut.script["status"] = []any{"PENDING"}
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("verdict = %s", res.Verdict)
	}
	if !strings.Contains(res.Reason, "PENDING") {
		t.Errorf("timeout error must name last observed value: %s", res.Reason)
	}
}

func TestCapturesAreScenarioGlobalAndLastWriteWins(t *testing.T) {
	sut, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: caps
setup:
  - { run: sut.submit, with: a, as: x }
method:
  - phase: p
    steps:
      - { run: sut.submit, with: b, as: x }
verify:
  - { run: assert, with: { of: "${.outputs.x.value}", equals: second }, desc: "last write wins" }
`)
	sut.script["submit"] = []any{"first", "second"}
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
}

func TestOnAbsentSkip(t *testing.T) {
	sut, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: skipper
verify:
  - { run: ghost.poke, onAbsent: skip, skipReason: "ghost not configured" }
  - { run: assert, with: { of: 1, equals: 1 } }
`)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	if res.Steps[0].Verdict != CheckSkip || res.Steps[0].SkipReason != "ghost not configured" {
		t.Fatalf("step = %+v", res.Steps[0])
	}
}

func TestKeepUpSkipsTeardown(t *testing.T) {
	sut, sc, reg := newWorld(t, passingScenario)
	sut.script["count"] = []any{1}
	rec := &Recorder{}
	RunScenario(context.Background(), sc, nil, reg, rec, Options{KeepUp: true})
	if sut.callCount("down") != 0 {
		t.Error("KEEP_UP must skip teardown")
	}
}

func TestExplicitTeardownReplacesDefault(t *testing.T) {
	sut, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: custom-td
verify:
  - { run: assert, with: { of: 1, equals: 1 } }
teardown:
  - { run: sut.kill, with: leftover }
`)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s", res.Verdict)
	}
	if sut.callCount("down") != 0 {
		t.Error("explicit teardown must replace the default docker.down")
	}
	if sut.callCount("kill") != 1 {
		t.Error("explicit teardown step must run")
	}
}

func TestBackgroundAndStopBackground(t *testing.T) {
	sut, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: bg
method:
  - phase: load
    steps:
      - { run: background, with: { name: load, step: { run: sut.status, with: x } } }
      - { run: stop_background, with: load, as: loadResult }
verify:
  - { run: assert, with: { of: "${.outputs.loadResult.value}", equals: ok } }
`)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
}

func TestTeardownDrainsUnstoppedBackground(t *testing.T) {
	sut, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: bgleak
method:
  - phase: load
    steps:
      - { run: background, with: { name: load, step: { run: sut.block } } }
verify:
  - { run: assert, with: { of: 1, equals: 1 } }
`)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	if got := sut.blockExits.Load(); got != 1 {
		t.Fatalf("unstopped background must be cancelled+awaited by run end; block exits = %d, want 1", got)
	}
}

func TestSnapshotScopeIsolatesBackgroundFromTimelineWrites(t *testing.T) {
	r := &runner{
		outputs: map[string]any{"x": 1},
		vars:    map[string]any{"v": 2},
		env:     map[string]any{"e": 3},
	}
	snap := r.snapshotScope()
	// timeline keeps mutating after the snapshot is taken
	r.outputs["x"] = 99
	r.outputs["y"] = "added later"
	r.vars["v"] = 88
	if snap.Outputs["x"] != 1 {
		t.Errorf("snapshot saw a later write to outputs: got %v, want 1", snap.Outputs["x"])
	}
	if _, ok := snap.Outputs["y"]; ok {
		t.Error("snapshot saw a key added after launch")
	}
	if snap.Vars["v"] != 2 {
		t.Errorf("snapshot saw a later write to vars: got %v, want 2", snap.Vars["v"])
	}
}

func TestWaitUntilBoundsProbeByTimeout(t *testing.T) {
	_, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: wuto
verify:
  - { run: wait_until, with: { probe: { run: sut.block }, timeout: 0.2, interval: 0.05, equals: ok } }
`)
	done := make(chan ScenarioResult, 1)
	go func() { done <- RunScenario(context.Background(), sc, nil, reg, &Recorder{}, Options{}) }()
	select {
	case res := <-done:
		if res.Verdict != ScenarioFailed {
			t.Fatalf("verdict = %s (%s); want FAILED on timeout", res.Verdict, res.Reason)
		}
		if !strings.Contains(res.Reason, "not observed within") {
			t.Errorf("reason = %q; want a wait_until timeout", res.Reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait_until did not respect its timeout — the blocking probe was not bounded")
	}
}

func TestDryRunSkipsActions(t *testing.T) {
	sut, sc, reg := newWorld(t, passingScenario)
	sut.script["count"] = []any{1}
	rec := &Recorder{}
	RunScenario(context.Background(), sc, nil, reg, rec, Options{DryRun: true, KeepUp: true})
	if sut.callCount("kill") != 0 || sut.callCount("up") != 0 {
		t.Error("dry-run must skip actions")
	}
	if sut.callCount("count") == 0 {
		t.Error("dry-run must still run probes")
	}
}

// Exercises execComposed: param binding, do: sequencing, macro-local capture
// via .outputs, last-step return, and a when:-false skip.
func TestComposedMacroExecution(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"project.yml": "apiVersion: shinari/v1\nkind: Project\nname: t\n",
		"providers/app.yml": `apiVersion: shinari/v1
kind: Provider
name: app
verbs:
  combine:
    params: [base, "extra?"]
    do:
      - { run: sut.echo, with: "${.params.base}", capture: { b: "." } }
      - { when: "${.params.extra}", run: sut.echo, with: "should-not-run" }
      - { run: sut.echo, with: "result-${.outputs.b}" }
`,
	}
	sut := &fakeSUT{script: map[string][]any{}, fails: map[string]error{}}
	currentSUT = sut
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, []byte(content), 0o644)
	}
	set, err := discover.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := registry.New(set, map[string]model.ProviderConfig{
		"sut": {Source: "fakesut"},
		"app": {Use: "./providers/app"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := model.ParseScenario([]byte(`
apiVersion: shinari/v1
kind: Scenario
name: composed
verify:
  - { run: app.combine, with: hello, as: out }
  - { run: assert, with: { of: "${.outputs.out.value}", equals: "result-hello" } }
`), "s.yml")
	if err != nil {
		t.Fatal(err)
	}
	res := RunScenario(context.Background(), sc, nil, reg, &Recorder{}, Options{KeepUp: true})
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	// steps 1 and 3 only; the when:-false step 2 is skipped
	if got := sut.callCount("echo"); got != 2 {
		t.Fatalf("echo called %d times; want 2 (the when:-false macro step must be skipped)", got)
	}
}

func TestPerStepEffectOverrideInjectsFault(t *testing.T) {
	sut, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: effoverride
method:
  - phase: degrade
    steps:
      - { run: sut.status, with: x, effect: outage, desc: "induced outage" }
verify:
  - { run: assert, with: { of: 1, equals: 1 } }
`)
	res, rec := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	if len(res.Injected) != 1 || res.Injected[0] != "induced outage" {
		t.Fatalf("injected = %v; want [induced outage]", res.Injected)
	}
	var faults int
	for _, e := range rec.Events {
		if e.Type == EvFaultInjected {
			faults++
		}
	}
	if faults != 1 {
		t.Fatalf("EvFaultInjected emitted %d times, want 1", faults)
	}
}

func TestPerStepKindOverrideSkippedInDryRun(t *testing.T) {
	sut, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: kindoverride
method:
  - phase: act
    steps:
      - { run: sut.echo, with: x, kind: action }
verify:
  - { run: assert, with: { of: 1, equals: 1 } }
`)
	RunScenario(context.Background(), sc, nil, reg, &Recorder{}, Options{DryRun: true, KeepUp: true})
	if got := sut.callCount("echo"); got != 0 {
		t.Fatalf("echo called %d times; a kind:action override must be skipped under dry-run", got)
	}
}

func TestEventStreamReducesToResult(t *testing.T) {
	sut, sc, reg := newWorld(t, passingScenario)
	sut.script["count"] = []any{1}
	rec := &Recorder{}
	res := RunScenario(context.Background(), sc, nil, reg, rec, Options{})
	reduced := Reduce(rec.Events)
	if len(reduced.Scenarios) != 1 {
		t.Fatalf("reduced %d scenarios", len(reduced.Scenarios))
	}
	got := reduced.Scenarios[0]
	// the reduction must rebuild the whole result from events, not just the verdict skeleton
	if !reflect.DeepEqual(got, res) {
		t.Errorf("reduced result is not a faithful reduction of the event stream:\n got:  %+v\n want: %+v", got, res)
	}
}

func TestRunClosesProvidersPerScenario(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"project.yml":       "apiVersion: shinari/v1\nkind: Project\nname: t\nproviders:\n  sut: { source: fakesut }\n",
		"scenarios/s/a.yml": "apiVersion: shinari/v1\nkind: Scenario\nname: a\nverify:\n  - { run: assert, with: { of: 1, equals: 1 } }\n",
		"scenarios/s/b.yml": "apiVersion: shinari/v1\nkind: Scenario\nname: b\nverify:\n  - { run: assert, with: { of: 1, equals: 1 } }\n",
	}
	sut := &fakeSUT{script: map[string][]any{}, fails: map[string]error{}}
	currentSUT = sut
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, []byte(content), 0o644)
	}
	set, err := discover.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), set, nil, nil, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := sut.closes.Load(); got != 2 {
		t.Fatalf("provider closed %d times across 2 scenarios, want 2", got)
	}
}

func TestRunTargets(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"project.yml":               "apiVersion: shinari/v1\nkind: Project\nname: t\nproviders:\n  sut: { source: fakesut }\n",
		"scenarios/data-loss/a.yml": "apiVersion: shinari/v1\nkind: Scenario\nname: a\nverify:\n  - { run: assert, with: { of: 1, equals: 1 } }\n",
		"scenarios/data-loss/b.yml": "apiVersion: shinari/v1\nkind: Scenario\nname: b\nverify:\n  - { run: assert, with: { of: 1, equals: 1 } }\n",
		"scenarios/net/c.yml":       "apiVersion: shinari/v1\nkind: Scenario\nname: c\nverify:\n  - { run: assert, with: { of: 1, equals: 1 } }\n",
	}
	sut := &fakeSUT{script: map[string][]any{}, fails: map[string]error{}}
	currentSUT = sut
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, []byte(content), 0o644)
	}
	set, err := discover.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	all, err := Run(context.Background(), set, nil, nil, Options{})
	if err != nil || len(all.Scenarios) != 3 {
		t.Fatalf("all: %d scenarios, %v", len(all.Scenarios), err)
	}
	bySuite, err := Run(context.Background(), set, []string{"data-loss"}, nil, Options{})
	if err != nil || len(bySuite.Scenarios) != 2 {
		t.Fatalf("suite: %d scenarios, %v", len(bySuite.Scenarios), err)
	}
	byName, err := Run(context.Background(), set, []string{"c"}, nil, Options{})
	if err != nil || len(byName.Scenarios) != 1 || byName.Scenarios[0].Name != "c" {
		t.Fatalf("name: %+v, %v", byName.Scenarios, err)
	}
	if _, err := Run(context.Background(), set, []string{"zzz"}, nil, Options{}); err == nil || !strings.Contains(err.Error(), "zzz") {
		t.Fatalf("unknown target must error listing known: %v", err)
	}
	if all.Verdict() != ScenarioPassed {
		t.Errorf("run verdict = %s", all.Verdict())
	}
}

// TestBindingsExposeMeta locks in that read: and capture: can reach a probe
// result's side-channels ($meta, $output), not just its value — so a check can
// gate on an HTTP status or container exit code that never lives in the body.
func TestBindingsExposeMeta(t *testing.T) {
	result := sdk.VerbResult{
		Value:  map[string]any{"id": "j-1"},
		Output: "raw-body",
		Meta:   map[string]any{"status": 403},
	}
	st := &model.Step{
		Read:    ".id",
		Capture: map[string]string{"code": "$meta.status", "raw": "$output"},
	}
	captured := map[string]any{}
	value, err := applyBindings(st, result, interp.Scope{}, func(n string, v any) { captured[n] = v })
	if err != nil {
		t.Fatal(err)
	}
	if value != "j-1" {
		t.Errorf("read value = %v, want j-1", value)
	}
	if captured["code"] != float64(403) {
		t.Errorf("captured code = %v (%T), want 403", captured["code"], captured["code"])
	}
	if captured["raw"] != "raw-body" {
		t.Errorf("captured raw = %v, want raw-body", captured["raw"])
	}
}

// TestReadInterpolatesScope pins that a read: jq expression is ${...}-
// interpolated through the scope before jq runs. ${.vars.want} is substituted
// as text, so the select matches; without interpolation jq treats it as a
// literal that never matches and silently yields 0.
func TestReadInterpolatesScope(t *testing.T) {
	result := sdk.VerbResult{Value: []any{
		map[string]any{"state": "SUCCESS"},
		map[string]any{"state": "FAILED"},
		map[string]any{"state": "SUCCESS"},
	}}
	scope := interp.Scope{Vars: map[string]any{"want": "SUCCESS"}}
	st := &model.Step{Read: `[.[] | select(.state=="${.vars.want}")] | length`}
	value, err := applyBindings(st, result, scope, func(string, any) {})
	if err != nil {
		t.Fatal(err)
	}
	if value != 2 {
		t.Errorf("read with ${.vars.want} = %v (%T), want 2", value, value)
	}
}

// TestCaptureInterpolatesScope covers the capture: site and the composed-verb
// case (${.params.X}), and proves the $meta jq variable still resolves on the
// same step — the textual ${...} pass leaves bare $name and \(...) untouched.
func TestCaptureInterpolatesScope(t *testing.T) {
	result := sdk.VerbResult{
		Value: []any{
			map[string]any{"state": "SUCCESS"},
			map[string]any{"state": "FAILED"},
		},
		Meta: map[string]any{"status": 200},
	}
	scope := interp.Scope{Params: map[string]any{"state": "FAILED"}}
	st := &model.Step{Capture: map[string]string{
		"failed": `[.[] | select(.state=="${.params.state}")] | length`,
		"code":   "$meta.status",
	}}
	captured := map[string]any{}
	if _, err := applyBindings(st, result, scope, func(n string, v any) { captured[n] = v }); err != nil {
		t.Fatal(err)
	}
	if captured["failed"] != 1 {
		t.Errorf("capture with ${.params.state} = %v, want 1", captured["failed"])
	}
	if captured["code"] != float64(200) {
		t.Errorf("$meta.status must still resolve alongside ${...}: %v", captured["code"])
	}
}

// TestWaitUntilReadInterpolatesScope pins the wait_until outer read: site:
// its read expression is interpolated through the scope before the poll loop.
func TestWaitUntilReadInterpolatesScope(t *testing.T) {
	sut, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: gated
vars: { want: RUNNING }
verify:
  - run: wait_until
    with:
      probe: { run: sut.status, with: j1 }
      read: 'if . == "${.vars.want}" then "hit" else "miss" end'
      equals: hit
      timeout: 1
      interval: 0.01
`)
	sut.script["status"] = []any{"PENDING", "RUNNING"}
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
}

// TestWaitUntilProbeReadInterpolatesScope pins the nested-step read: site
// (execStepMap): the read inside a wait_until probe is interpolated too.
func TestWaitUntilProbeReadInterpolatesScope(t *testing.T) {
	sut, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: gated
vars: { want: RUNNING }
verify:
  - run: wait_until
    with:
      probe:
        run: sut.status
        with: j1
        read: 'if . == "${.vars.want}" then "hit" else "miss" end'
      equals: hit
      timeout: 1
      interval: 0.01
`)
	sut.script["status"] = []any{"PENDING", "RUNNING"}
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
}

// TestWhenGuardSkipsAndRuns covers the value-gated guard: a falsey when: skips
// the step (SKIP, scenario unaffected), a truthy one runs it. The predicate is
// a jq expression over the scope, including a runtime-captured output.
func TestWhenGuardSkipsAndRuns(t *testing.T) {
	sut, sc, reg := newWorld(t, `
apiVersion: shinari/v1
kind: Scenario
name: guarded
vars: { n: 1 }
setup:
  - { run: sut.up, with: [app] }
verify:
  - { run: sut.count, with: sleep, as: total }
  - { run: assert, when: "${.outputs.total.value > 1}", with: { of: 1, equals: 2 }, desc: "guarded-off" }
  - { run: assert, when: "${.vars.n > 0}", with: { of: 1, equals: 1 }, desc: "guarded-on" }
`)
	sut.script["count"] = []any{1}
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	byDesc := map[string]StepResult{}
	for _, s := range res.Steps {
		if s.Desc != "" {
			byDesc[s.Desc] = s
		}
	}
	if got := byDesc["guarded-off"]; got.Verdict != CheckSkip {
		t.Errorf("guarded-off verdict = %s, want SKIP (the failing assert must not run)", got.Verdict)
	}
	if got := byDesc["guarded-on"]; got.Verdict != CheckPass {
		t.Errorf("guarded-on verdict = %s, want PASS", got.Verdict)
	}
}
