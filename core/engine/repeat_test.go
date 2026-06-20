// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"strings"
	"testing"
)

func TestRepeatRunsBodyNTimes(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: repeat-basic
verify:
  - run: repeat
    with:
      times: 3
      do:
        - { run: sut.submit, with: a }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	if sut.callCount("submit") != 3 {
		t.Errorf("body must run 3 times, submit called %d", sut.callCount("submit"))
	}
}

func TestRepeatTimesMustBePositive(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: repeat-zero
verify:
  - { run: repeat, with: { times: 0, do: [ { run: sut.submit, with: a } ] } }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("verdict = %s, want FAILED", res.Verdict)
	}
	if !strings.Contains(res.Reason, "times must be >= 1") {
		t.Errorf("reason = %q", res.Reason)
	}
}

func TestRepeatEmptyDoIsAFailure(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: repeat-empty
verify:
  - { run: repeat, with: { times: 2, do: [] } }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("verdict = %s, want FAILED", res.Verdict)
	}
	if !strings.Contains(res.Reason, "non-empty") {
		t.Errorf("reason = %q, want it to mention a non-empty do", res.Reason)
	}
}

func TestRepeatFailFastStopsAtFirstFailure(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: repeat-failfast
verify:
  - run: repeat
    with:
      times: 5
      do:
        - { run: sut.status, with: x, as: s }
        - { run: assert, with: { of: "${.outputs.s.value}", equals: ok } }
`
	sut, sc, reg := newWorld(t, src)
	sut.script["status"] = []any{"ok", "bad"} // iter1 ok, iter2 bad (last repeats)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("verdict = %s, want FAILED", res.Verdict)
	}
	if sut.callCount("status") != 2 {
		t.Errorf("fail-fast must stop after iteration 2, status called %d", sut.callCount("status"))
	}
}

func TestRepeatRunAllWhenStopOnFailFalse(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: repeat-runall
verify:
  - run: repeat
    with:
      times: 3
      stopOnFail: false
      do:
        - { run: sut.status, with: x, as: s }
        - { run: assert, with: { of: "${.outputs.s.value}", equals: ok } }
`
	sut, sc, reg := newWorld(t, src)
	sut.script["status"] = []any{"ok", "bad"} // iter1 ok, then bad forever
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("verdict = %s, want FAILED", res.Verdict)
	}
	if sut.callCount("status") != 3 {
		t.Errorf("run-all must run all 3 iterations, status called %d", sut.callCount("status"))
	}
}

func TestRepeatCapturesCarryForwardAndPersist(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: repeat-caps
verify:
  - run: repeat
    with:
      times: 3
      do:
        - { run: sut.submit, with: a, as: last }
  - { run: assert, with: { of: "${.outputs.last.value}", equals: third }, desc: "last iteration's capture persists" }
`
	sut, sc, reg := newWorld(t, src)
	sut.script["submit"] = []any{"first", "second", "third"}
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
}

func TestRepeatTracksFaultInjectionPerIteration(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: repeat-flap
method:
  - phase: "flap redis"
    steps:
      - run: repeat
        with:
          times: 3
          do:
            - { run: sut.kill, with: redis }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if len(res.Injected) != 3 {
		t.Fatalf("each cycle's kill must be tracked as injected, got %v", res.Injected)
	}
}

func TestRepeatDryRunRunsBodyOnce(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: repeat-dryrun
verify:
  - run: repeat
    with:
      times: 5
      do:
        - { run: sut.kill, with: redis }
        - { run: sut.count }
`
	sut, sc, reg := newWorld(t, src)
	rec := &Recorder{}
	res := RunScenario(context.Background(), sc, nil, reg, rec, Options{DryRun: true, KeepUp: true})
	if res.Verdict == ScenarioErrored {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	if sut.callCount("kill") != 0 {
		t.Errorf("dry-run must skip the action, kill called %d", sut.callCount("kill"))
	}
	if sut.callCount("count") != 1 {
		t.Errorf("dry-run must run the body exactly once, count probe called %d", sut.callCount("count"))
	}
}

func TestRepeatPairedBackgroundAcrossIterations(t *testing.T) {
	// A background started AND stopped within each iteration must not collide
	// across iterations.
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: repeat-bg-paired
method:
  - phase: cycle
    steps:
      - run: repeat
        with:
          times: 3
          do:
            - { run: background, with: { name: gen, step: { run: sut.status, with: x } } }
            - { run: stop_background, with: gen }
verify:
  - { run: assert, with: { of: 1, equals: 1 } }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	if sut.callCount("status") < 3 {
		t.Errorf("each iteration's background must run, status called %d", sut.callCount("status"))
	}
}

func TestRepeatFailFastCancelsStartedBackground(t *testing.T) {
	// Iteration 1 starts a long-running background, then fails BEFORE its
	// stop_background. The repeat must cancel + remove that background, so a
	// later stop_background for the same name reports it gone. Placed in verify
	// (which runs all steps) because a failed method phase returns immediately.
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: repeat-bg-cleanup
verify:
  - run: repeat
    with:
      times: 2
      do:
        - { run: background, with: { name: gen, step: { run: sleep, with: { seconds: 30 } } } }
        - { run: assert, with: { of: 1, equals: 2 } }
        - { run: stop_background, with: gen }
  - { run: stop_background, with: gen }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("verdict = %s, want FAILED", res.Verdict)
	}
	// the trailing standalone stop_background must error: the background was
	// cancelled and removed by the aborted iteration. (The body's own
	// stop_background never ran — fail-fast aborted iteration 1 first.)
	var stop *StepResult
	for i := range res.Steps {
		if res.Steps[i].Run == "stop_background" {
			stop = &res.Steps[i]
		}
	}
	if stop == nil {
		t.Fatal("no stop_background step ran")
	}
	if stop.Verdict != CheckFail || !strings.Contains(stop.Err, "no background task named") {
		t.Fatalf("expected stop_background to fail 'no background task named', got %+v", stop)
	}
}

func TestRepeatResultEqualsReduce(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: repeat-reduce
verify:
  - run: repeat
    with:
      times: 2
      do:
        - { run: sut.submit, with: a }
        - { run: sut.status, with: a }
`
	sut, sc, reg := newWorld(t, src)
	res, rec := run(t, sut, sc, reg)
	reduced := Reduce(rec.Events)
	if len(reduced.Scenarios) != 1 {
		t.Fatalf("Reduce produced %d scenarios", len(reduced.Scenarios))
	}
	if got, want := len(reduced.Scenarios[0].Steps), len(res.Steps); got != want {
		t.Fatalf("Reduce step count = %d, live = %d", got, want)
	}
	for i := range res.Steps {
		if reduced.Scenarios[0].Steps[i].Verdict != res.Steps[i].Verdict {
			t.Errorf("step %d: reduced %s != live %s", i,
				reduced.Scenarios[0].Steps[i].Verdict, res.Steps[i].Verdict)
		}
	}
}

func TestRepeatNestsInsideParallel(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: parallel-gt-repeat
verify:
  - run: parallel
    with:
      branches:
        - - { run: sut.submit, with: outer }
        - - run: repeat
            with:
              times: 2
              do:
                - { run: sut.submit, with: inner }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	if sut.callCount("submit") != 3 { // 1 outer + 2 inner
		t.Errorf("parallel>repeat must run 3 submits, got %d", sut.callCount("submit"))
	}
}

func TestParallelNestsInsideRepeat(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: repeat-gt-parallel
verify:
  - run: repeat
    with:
      times: 2
      do:
        - run: parallel
          with:
            branches:
              - - { run: sut.submit, with: a }
              - - { run: sut.submit, with: b }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	if sut.callCount("submit") != 4 { // 2 iterations x 2 branches
		t.Errorf("repeat>parallel must run 4 submits, got %d", sut.callCount("submit"))
	}
}
