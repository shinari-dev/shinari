// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/shinari-dev/shinari/core/engine"
)

func stepStream() []engine.Event {
	return []engine.Event{
		{Type: engine.EvScenarioStarted, Scenario: "s"},
		{Type: engine.EvStepStarted, Scenario: "s", Section: "verify", Step: "sut.status", Verb: "sut.status"},
		{Type: engine.EvStepPassed, Scenario: "s", Section: "verify", Step: "sut.status",
			Payload: map[string]any{"verdict": "PASS", "value": "RUNNING", "durationMs": int64(12)}},
	}
}

func TestConsoleVerboseShowsValueAndDuration(t *testing.T) {
	var buf bytes.Buffer
	c := &Console{W: &buf, Verbose: true}
	for _, e := range stepStream() {
		c.Emit(e)
	}
	out := buf.String()
	if !strings.Contains(out, "verify") {
		t.Errorf("missing phase label:\n%s", out)
	}
	if !strings.Contains(out, "→ RUNNING") || !strings.Contains(out, "(12ms)") {
		t.Errorf("missing value/duration:\n%s", out)
	}
}

func TestConsoleNonVerboseUnchanged(t *testing.T) {
	var buf bytes.Buffer
	c := &Console{W: &buf} // Verbose defaults false
	for _, e := range stepStream() {
		c.Emit(e)
	}
	out := buf.String()
	if strings.Contains(out, "~ verify") || strings.Contains(out, "RUNNING") || strings.Contains(out, "ms)") {
		t.Errorf("non-verbose output leaked verbose detail:\n%s", out)
	}
	if !strings.Contains(out, "✓ sut.status") {
		t.Errorf("non-verbose missing the plain step line:\n%s", out)
	}
}
