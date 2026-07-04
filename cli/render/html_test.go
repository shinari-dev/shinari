// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/shinari-dev/shinari/core/engine"
)

func TestHTMLShape(t *testing.T) {
	var buf bytes.Buffer
	if err := HTML(&buf, sample()); err != nil {
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
		"<b>1</b><span>scenario</span>", // singular counts stay singular
		"<b>1</b><span>finding</span>",
		"3 steps",
		`class="copy" data-i="0"`,                       // per-scenario copy-for-LLM button
		"## worker-killed",                              // markdown payload for the LLM
		"| section | check | verdict |",                 // markdown step table
		`href="https://shinari.dev/"`,                   // docs link
		`href="https://github.com/shinari-dev/shinari"`, // github link
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
	if err := HTML(&buf, res); err != nil {
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
	if err := HTML(&buf, res); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "now passes: promote to a hard assertion") {
		t.Errorf("now-passes finding not flagged for promotion")
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
