// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"testing"
)

// TestStepEventsCarryValueAndDuration locks in the verbose/journal contract:
// a passing step emits its produced value and a durationMs on the event.
func TestStepEventsCarryValueAndDuration(t *testing.T) {
	sut, sc, reg := newWorld(t, passingScenario)
	sut.script["count"] = []any{1}
	rec := &Recorder{}
	RunScenario(context.Background(), sc, nil, reg, rec, Options{})

	var found bool
	for _, e := range rec.Events {
		if e.Type != EvStepPassed || e.Verb != "sut.count" {
			continue
		}
		found = true
		if _, ok := e.Payload["durationMs"]; !ok {
			t.Errorf("step.passed for sut.count missing durationMs: %+v", e.Payload)
		}
		if e.Payload["value"] != 1 {
			t.Errorf("step.passed value = %v, want 1", e.Payload["value"])
		}
	}
	if !found {
		t.Fatal("no step.passed event for sut.count")
	}
}
