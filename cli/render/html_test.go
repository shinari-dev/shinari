// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shinari-dev/shinari/core/engine"
)

func TestHTMLShape(t *testing.T) {
	var buf bytes.Buffer
	if err := HTML(&buf, sample(), "9.9.9-test"); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	for _, want := range []string{
		"<!doctype html",
		"<title>Shinari report — PASSED</title>",
		`class="badge v-PASSED"`,
		"worker-killed",
		"data-loss",
		"docker.kill worker-a",        // Injected
		"exactly once",                // Held
		"connections leak after kill", // Gapped
		"observed: expected 0 == 12",
		`class="c-FINDING"`,
		`class="badge v-FINDING"`, // passed-with-findings tag next to the verdict
		">FINDING</span>",
		"<b>1</b><span>scenario</span>", // singular counts stay singular
		"<b>1</b><span>finding</span>",
		"3 steps",
		`class="copy" data-i="0"`,                       // per-scenario copy-for-LLM button
		"## worker-killed",                              // markdown payload for the LLM
		"| section | check | verdict |",                 // markdown step table
		`href="https://shinari.dev/"`,                   // docs link
		`href="https://github.com/shinari-dev/shinari"`, // github link
		"v9.9.9-test",                                   // version stamp in the footer
	} {
		if !strings.Contains(s, want) {
			t.Errorf("report.html missing %q", want)
		}
	}
	// A passed scenario collapses (no open attribute on its <details>).
	if strings.Contains(s, `class="scenario" open>`) {
		t.Errorf("passed scenario should not be expanded:\n%s", s)
	}
}

func TestHTMLEscapesAndExpandsFailures(t *testing.T) {
	t0 := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	res := engine.RunResult{
		Start: t0, End: t0.Add(time.Minute),
		Scenarios: []engine.ScenarioResult{{
			Name: "evil", Verdict: engine.ScenarioFailed,
			Reason: `assert "<script>alert(1)</script>" failed`,
			Start:  t0, End: t0.Add(time.Second),
			Steps: []engine.StepResult{{
				Section: "verify", Run: "assert", Verdict: engine.CheckFail,
				Err: "expected <b> == 1", Start: t0, End: t0.Add(time.Second),
			}},
		}},
	}
	var buf bytes.Buffer
	if err := HTML(&buf, res, "9.9.9-test"); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if strings.Contains(s, "<script>alert(1)</script>") || strings.Contains(s, "expected <b>") {
		t.Errorf("unescaped user content in report:\n%s", s)
	}
	if !strings.Contains(s, "&lt;script&gt;") {
		t.Errorf("reason not rendered escaped")
	}
	if !strings.Contains(s, `class="scenario" open>`) {
		t.Errorf("failed scenario should be expanded")
	}
	if !strings.Contains(s, `class="badge v-FAILED"`) {
		t.Errorf("failed badge missing")
	}
}

func TestHTMLNowPassesPromotion(t *testing.T) {
	res := sample()
	res.Scenarios[0].Findings[0].NowPasses = true
	var buf bytes.Buffer
	if err := HTML(&buf, res, "9.9.9-test"); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "now passes: promote to a hard assertion") {
		t.Errorf("now-passes finding not flagged for promotion")
	}
	// A now-passing finding is a promotion prompt, not an open gap, so the
	// passed-with-findings tag must not appear.
	if strings.Contains(s, `class="badge v-FINDING"`) {
		t.Errorf("now-passes finding should not raise the FINDING tag")
	}
}

func TestHTMLTruncatesLongLogs(t *testing.T) {
	t0 := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	var log strings.Builder
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&log, "line %d: connection refused\n", i)
	}
	res := engine.RunResult{
		Start: t0, End: t0.Add(time.Minute),
		Scenarios: []engine.ScenarioResult{{
			Name: "noisy", Verdict: engine.ScenarioFailed,
			Start: t0, End: t0.Add(time.Second),
			Steps: []engine.StepResult{{
				Section: "verify", Run: "assert", Verdict: engine.CheckFail,
				Err: log.String(), Start: t0, End: t0.Add(time.Second),
			}},
		}},
	}
	var buf bytes.Buffer
	if err := HTML(&buf, res, "9.9.9-test"); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, `<details class="logs">`) {
		t.Errorf("long log not wrapped in a collapsible details")
	}
	if !strings.Contains(s, "view full logs (121 lines)") {
		t.Errorf("toggle missing the line count:\n%s", s)
	}
	// Isolate the collapsed preview and assert it shows the first lines but
	// stops before line 3 (the 4th line) — that only lives in the full log.
	_, rest, _ := strings.Cut(s, `<pre class="preview">`)
	preview, _, _ := strings.Cut(rest, `</pre>`)
	if !strings.Contains(preview, "line 0: connection refused") {
		t.Errorf("preview should include the first line, got %q", preview)
	}
	if strings.Contains(preview, "line 3: connection refused") {
		t.Errorf("preview should stop after %d lines, got %q", 3, preview)
	}
	if !strings.Contains(s, `<pre class="full">`) || !strings.Contains(s, "line 119: connection refused") {
		t.Errorf("full log should carry every line")
	}
}

func TestHTMLTruncatesScenarioReason(t *testing.T) {
	t0 := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	var reason strings.Builder
	reason.WriteString("healthcheck failed:\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&reason, "  attempt %d timed out\n", i)
	}
	res := engine.RunResult{
		Start: t0, End: t0.Add(time.Minute),
		Scenarios: []engine.ScenarioResult{{
			Name: "flaky", Verdict: engine.ScenarioFailed, Reason: reason.String(),
			Start: t0, End: t0.Add(time.Second),
		}},
	}
	var buf bytes.Buffer
	if err := HTML(&buf, res, "9.9.9-test"); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	// The failure reason under the scenario description gets the same toggle,
	// not just the step-table detail.
	if !strings.Contains(s, `<div class="reason"><details class="logs">`) {
		t.Errorf("long scenario reason not wrapped in a collapsible details:\n%s", s)
	}
}

func TestClampDetail(t *testing.T) {
	if _, long, _ := clampDetail("short\ntwo lines"); long {
		t.Errorf("a two-line detail should not be treated as long")
	}
	preview, long, n := clampDetail("a\nb\nc\nd\ne")
	if !long || n != 5 {
		t.Errorf("5-line detail: long=%v lines=%d, want true/5", long, n)
	}
	if preview != "a\nb\nc" {
		t.Errorf("preview = %q, want first 3 lines", preview)
	}
}

func TestFmtDur(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Microsecond, "<1ms"},
		{250 * time.Millisecond, "250ms"},
		{1500 * time.Millisecond, "1.5s"},
		{90 * time.Second, "1m30s"},
	} {
		if got := fmtDur(tc.d); got != tc.want {
			t.Errorf("fmtDur(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
