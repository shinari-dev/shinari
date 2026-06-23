// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"strings"
	"testing"
)

func TestParallelRunsAllBranches(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: parallel-basic
verify:
  - run: parallel
    with:
      branches:
        - - { run: sut.submit, with: a }
        - - { run: sut.submit, with: b }
          - { run: sut.status, with: x }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	if sut.callCount("submit") != 2 {
		t.Errorf("both branches must run their submit, got %d", sut.callCount("submit"))
	}
	if sut.callCount("status") != 1 {
		t.Errorf("branch 1 status must run, got %d", sut.callCount("status"))
	}
}

func TestParallelFailsWhenABranchAssertionFails(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: parallel-fail
verify:
  - run: parallel
    with:
      branches:
        - - { run: sut.submit, with: a }
        - - { run: sut.status, as: s }
          - { run: assert, with: { of: "${.outputs.s.value}", equals: "never" } }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("verdict = %s, want FAILED (branch assertion failed)", res.Verdict)
	}
	// the sibling action branch still ran to completion (no cancellation)
	if sut.callCount("submit") != 1 {
		t.Errorf("sibling branch must still complete, submit ran %d times", sut.callCount("submit"))
	}
}

func TestParallelEmptyBranchesIsAFailure(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: parallel-empty
verify:
  - { run: parallel, with: { branches: [] } }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("verdict = %s, want FAILED", res.Verdict)
	}
	if !strings.Contains(res.Reason, "non-empty") {
		t.Errorf("reason = %q, want it to mention non-empty branches", res.Reason)
	}
}

func TestParallelBranchFindingKeepsScenarioGreen(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: parallel-finding
verify:
  - run: parallel
    with:
      branches:
        - - { run: sut.submit, with: a }
        - - { run: sut.status, as: s }
          - run: assert
            with: { of: "${.outputs.s.value}", equals: "never" }
            finding: "known gap: status not yet 'never'"
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s); a branch finding must keep the scenario green", res.Verdict, res.Reason)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("expected 1 finding recorded, got %d", len(res.Findings))
	}
}

func TestParallelBranchFaultIsInjected(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: parallel-inject
method:
  - phase: "concurrent-faults"
    steps:
      - run: parallel
        with:
          branches:
            - - { run: sut.kill, with: node-a }
            - - { run: sut.kill, with: node-b }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if len(res.Injected) != 2 {
		t.Fatalf("both concurrent kills must be tracked as injected, got %v", res.Injected)
	}
}

func TestParallelBranchBackgroundIsStoppableAfterBarrier(t *testing.T) {
	// A fault verb implemented via `background` (e.g. netem.delay) is the
	// documented way to inject mid-load: started in one parallel branch while a
	// sibling drives load. Its handle must survive the barrier so a later
	// stop_background (netem.clear, in the next phase) can find and stop it.
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: parallel-bg-survives
method:
  - phase: "inject in a branch"
    steps:
      - run: parallel
        with:
          branches:
            - - { run: sut.submit, with: load }
            - - run: background
                with:
                  name: fault
                  step: { run: sleep, with: { seconds: 30 } }
  - phase: "clear the fault in a later phase"
    steps:
      - run: stop_background
        with: fault
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s); a branch background must be stoppable after the barrier", res.Verdict, res.Reason)
	}
}

func TestParallelIsDeterministic(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: parallel-determinism
verify:
  - run: parallel
    with:
      branches:
        - - { run: sut.submit, with: a }
          - { run: sut.status, with: a }
        - - { run: sut.submit, with: b }
        - - { run: sut.status, with: c }
          - { run: sut.submit, with: c }
`
	// Run the same scenario twice; the ordered step verdicts and the event
	// type/verb sequence must be byte-identical (branch-order flush, not race
	// order). Timestamps are not compared.
	stepsOf := func() ([]string, []string) {
		sut, sc, reg := newWorld(t, src)
		res, rec := run(t, sut, sc, reg)
		var steps []string
		for _, s := range res.Steps {
			steps = append(steps, s.Section+"/"+s.Run+"="+string(s.Verdict))
		}
		var events []string
		for _, e := range rec.Events {
			events = append(events, string(e.Type)+":"+e.Verb)
		}
		return steps, events
	}

	for i := 0; i < 20; i++ {
		s1, e1 := stepsOf()
		s2, e2 := stepsOf()
		if strings.Join(s1, ",") != strings.Join(s2, ",") {
			t.Fatalf("step order not deterministic:\n run A: %v\n run B: %v", s1, s2)
		}
		if strings.Join(e1, ",") != strings.Join(e2, ",") {
			t.Fatalf("event order not deterministic:\n run A: %v\n run B: %v", e1, e2)
		}
	}
}

func TestParallelResultEqualsReduce(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: parallel-reduce
verify:
  - run: parallel
    with:
      branches:
        - - { run: sut.submit, with: a }
        - - { run: sut.status, with: b }
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
		if reduced.Scenarios[0].Steps[i].Run != res.Steps[i].Run ||
			reduced.Scenarios[0].Steps[i].Verdict != res.Steps[i].Verdict {
			t.Errorf("step %d: reduced %s/%s != live %s/%s", i,
				reduced.Scenarios[0].Steps[i].Run, reduced.Scenarios[0].Steps[i].Verdict,
				res.Steps[i].Run, res.Steps[i].Verdict)
		}
	}
}

func TestParallelCaptureMergeHigherIndexWins(t *testing.T) {
	// sut.echo returns its primary arg as the value, so the winning branch is
	// value-checkable regardless of concurrent call order. Both branches bind
	// `shared`; the higher-indexed branch must win, deterministically.
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: parallel-capture
verify:
  - run: parallel
    with:
      branches:
        - - { run: sut.echo, with: low, as: shared }
        - - { run: sut.echo, with: high, as: shared }
  - { run: assert, with: { of: "${.outputs.shared.value}", equals: "high" } }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s); higher-indexed branch must win the capture", res.Verdict, res.Reason)
	}
}

func TestParallelNests(t *testing.T) {
	const src = `
apiVersion: shinari/v1
kind: Scenario
name: parallel-nested
verify:
  - run: parallel
    with:
      branches:
        - - { run: sut.submit, with: outer-a }
        - - run: parallel
            with:
              branches:
                - - { run: sut.submit, with: inner-a }
                - - { run: sut.submit, with: inner-b }
`
	sut, sc, reg := newWorld(t, src)
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioPassed {
		t.Fatalf("verdict = %s (%s)", res.Verdict, res.Reason)
	}
	if sut.callCount("submit") != 3 {
		t.Errorf("outer + 2 inner submits = 3, got %d", sut.callCount("submit"))
	}
}

func TestLiveBranchCapErrors(t *testing.T) {
	// Build a parallel step whose branch count far exceeds the cap, so enough
	// branches overlap to trip the limiter even on a fast machine. Each branch
	// sleeps briefly to guarantee concurrent overlap.
	var sb strings.Builder
	sb.WriteString("apiVersion: shinari/v1\nkind: Scenario\nname: cap\nverify:\n  - run: parallel\n    with:\n      branches:\n")
	for i := 0; i < maxLiveBranches*4; i++ {
		sb.WriteString("        - - { run: sleep, with: { seconds: 0.2 } }\n")
	}
	sut, sc, reg := newWorld(t, sb.String())
	res, _ := run(t, sut, sc, reg)
	if res.Verdict != ScenarioFailed {
		t.Fatalf("verdict = %s, want FAILED (cap exceeded)", res.Verdict)
	}
	if !strings.Contains(res.Reason, "live-branch cap") {
		t.Errorf("reason = %q, want it to mention the live-branch cap", res.Reason)
	}
}
