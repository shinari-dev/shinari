// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package otelx

import (
	"context"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/shinari-dev/shinari/core/engine"
)

func TestBuildSpansTree(t *testing.T) {
	t0 := time.Unix(1000, 0)
	res := engine.RunResult{
		Start: t0, End: t0.Add(3 * time.Second),
		Scenarios: []engine.ScenarioResult{{
			Name: "checkout", Verdict: engine.ScenarioPassed,
			Start: t0, End: t0.Add(2 * time.Second),
			Steps: []engine.StepResult{
				{Section: "verify", Run: "assert", Desc: "exactly once", Verdict: engine.CheckPass,
					Start: t0, End: t0.Add(time.Second)},
			},
			Findings: []engine.FindingRecord{
				{ID: "sha-abc", Scenario: "checkout", Narrative: "gap"},
			},
		}},
	}

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	BuildSpans(context.Background(), tp.Tracer("test"), res)

	names := map[string]bool{}
	var foundFindingEvent bool
	for _, s := range sr.Ended() {
		names[s.Name()] = true
		for _, e := range s.Events() {
			if e.Name == "finding" {
				foundFindingEvent = true
			}
		}
	}
	for _, want := range []string{"shinari.run", "scenario:checkout", "verify/exactly once"} {
		if !names[want] {
			t.Fatalf("missing span %q; got %v", want, names)
		}
	}
	if !foundFindingEvent {
		t.Fatal("expected a 'finding' span event on the scenario span")
	}
}
