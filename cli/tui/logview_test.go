// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/shinari-dev/shinari/core/engine"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func TestRenderLogLinePerEvent(t *testing.T) {
	evs := []engine.Event{
		{Type: engine.EvStepStarted, Time: time.Unix(0, 0).UTC(), Step: "kill redis", Verb: "redis.kill"},
		{Type: engine.EvStepFailed, Time: time.Unix(6, 0).UTC(), Step: "recovery", Payload: map[string]any{"verdict": "FAILED", "error": "6.2s > 5s"}},
	}
	lines := RenderLog(evs)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "step.started") || !strings.Contains(lines[0], "redis.kill") {
		t.Fatalf("line 0 missing type/verb: %q", lines[0])
	}
	if !strings.Contains(lines[1], "6.2s > 5s") {
		t.Fatalf("line 1 missing payload detail: %q", lines[1])
	}
}

// Each log line opens with a right-aligned line-number gutter, padded to the
// width of the largest number so the timestamps stay aligned.
func TestRenderLogNumbersLines(t *testing.T) {
	evs := make([]engine.Event, 12)
	for i := range evs {
		evs[i] = engine.Event{Type: engine.EvStepStarted, Time: time.Unix(0, 0).UTC()}
	}
	lines := RenderLog(evs)
	if got := stripANSI(lines[0]); !strings.HasPrefix(got, " 1  ") {
		t.Fatalf("line 1 should start with a width-2 right-aligned number, got %q", got)
	}
	if got := stripANSI(lines[11]); !strings.HasPrefix(got, "12  ") {
		t.Fatalf("line 12 should start with its number, got %q", got)
	}
}
