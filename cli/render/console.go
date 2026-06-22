// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package render turns the core Result/event contract into every CLI
// output: console stream, results.tsv, results.json, junit.xml, JSON
// journal, findings report. Rendering is exclusively a front-end concern
// — nothing here reaches into engine internals.
package render

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shinari-dev/shinari/core/engine"
	"github.com/shinari-dev/shinari/utils/conv"
)

// phaseCol is the width of the left-hand lifecycle-phase column. It is wide
// enough for the longest label ("teardown") plus a space of breathing room.
const phaseCol = 10

// Console is a streaming renderer: it subscribes to the live event stream and
// lays each scenario out as its resilience arc — steps grouped under the
// lifecycle phase (setup, steady, method, recovery, verify, teardown) they ran
// in, with the injected fault and any held findings called out. Verbose adds
// the value each step produced, its duration, and gate observations.
type Console struct {
	W       io.Writer
	Verbose bool
	Pal     Palette

	// per-scenario streaming state, reset on EvScenarioStarted.
	phase   string    // last phase label printed (dedup)
	started time.Time // scenario start, for the verdict-line duration
	held    int       // findings held so far
	finding string    // narrative from the last EvFindingRecorded, consumed by its FINDING step line
}

func (c *Console) Emit(e engine.Event) {
	p := c.Pal
	switch e.Type {
	case engine.EvScenarioStarted:
		c.phase, c.held, c.finding, c.started = "", 0, "", e.Time
		fill := 56 - len(e.Scenario)
		if fill < 3 {
			fill = 3
		}
		fmt.Fprintf(c.W, "\n%s %s %s\n", p.Bold("━━"), p.Bold(e.Scenario), p.Dim(strings.Repeat("─", fill)))

	case engine.EvStepPassed:
		fmt.Fprintf(c.W, "%s%s %s%s\n", c.label(e), p.Pass("✓"), e.Step, c.detail(e))

	case engine.EvStepFailed:
		if e.Payload["verdict"] == string(engine.CheckFinding) {
			c.findingLine(e)
			return
		}
		fmt.Fprintf(c.W, "%s%s %s %s %v%s\n",
			c.label(e), p.Fail("✗"), p.Fail(e.Step), p.Dim("—"), e.Payload["error"], c.detail(e))

	case engine.EvStepSkipped:
		fmt.Fprintf(c.W, "%s%s %s\n", c.label(e), p.Skip("↷"), p.Skip(e.Step+" (skipped)"))

	case engine.EvFaultInjected:
		fmt.Fprintf(c.W, "%s%s %s %s\n", c.label(e), p.Fault("⚡"), e.Verb, p.Fault("(fault injected)"))

	case engine.EvGateObserved:
		if c.Verbose {
			fmt.Fprintf(c.W, "%s%s gate observed: %v\n", c.pad(), p.Gate("▷"), e.Payload["observed"])
		}

	case engine.EvFindingRecorded:
		// A finding that now passes is reported by its promote-to-FAIL step
		// line; only a still-failing finding is held and annotated here.
		if e.Payload["nowPasses"] == true {
			return
		}
		c.held++
		if n, ok := e.Payload["narrative"].(string); ok {
			c.finding = n
		}

	case engine.EvScenarioFinished:
		c.verdictLine(e)
	}
}

// findingLine renders a held finding: the check that failed, annotated with the
// narrative captured from the preceding EvFindingRecorded.
func (c *Console) findingLine(e engine.Event) {
	p := c.Pal
	narr := c.finding
	c.finding = ""
	tail := p.Finding("FINDING")
	if narr != "" {
		tail = p.Dim("· ") + p.Finding("FINDING: "+narr)
	}
	fmt.Fprintf(c.W, "%s%s %s %s\n", c.label(e), p.Finding("◆"), e.Step, tail)
}

// verdictLine closes a scenario with its colored verdict badge, the count of
// findings held, the elapsed time, and (when not passing) the reason.
func (c *Console) verdictLine(e engine.Event) {
	p := c.Pal
	v, _ := e.Payload["verdict"].(string)
	reason, _ := e.Payload["reason"].(string)
	head := verdictGlyph(p, v) + " " + p.Verdict(v)

	var meta []string
	if c.held > 0 {
		meta = append(meta, fmt.Sprintf("%d finding%s held", c.held, plural(c.held)))
	}
	if !c.started.IsZero() && !e.Time.IsZero() {
		meta = append(meta, e.Time.Sub(c.started).Round(time.Millisecond).String())
	}
	if len(meta) > 0 {
		head += " " + p.Dim("· "+strings.Join(meta, " · "))
	}
	fmt.Fprintf(c.W, "\n  %s\n", head)
	if v != string(engine.ScenarioPassed) && reason != "" {
		fmt.Fprintf(c.W, "    %s %s\n", p.Dim("↳"), reason)
	}
}

// label returns the left-column prefix for a step line: two spaces of indent
// plus the lifecycle-phase label, printed (dimmed) only the first time a phase
// is entered and padded to a blank column on every following line in it.
func (c *Console) label(e engine.Event) string {
	lbl := phaseLabel(e.Section)
	if lbl == c.phase {
		return c.pad()
	}
	c.phase = lbl
	return "  " + c.Pal.Dim(fmt.Sprintf("%-*s", phaseCol, lbl))
}

func (c *Console) pad() string { return "  " + strings.Repeat(" ", phaseCol) }

// detail is the verbose suffix on a step line: the value it produced (truncated)
// and how long it took. Empty unless verbose.
func (c *Console) detail(e engine.Event) string {
	if !c.Verbose {
		return ""
	}
	p := c.Pal
	var parts []string
	if v, ok := e.Payload["value"]; ok && v != nil {
		if s := strings.TrimSpace(conv.ToString(v)); s != "" {
			parts = append(parts, p.Dim("→ ")+conv.Truncate(s, 60))
		}
	}
	if ms, ok := e.Payload["durationMs"]; ok {
		parts = append(parts, p.Dim(fmt.Sprintf("(%vms)", ms)))
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, " ")
}

// phaseLabel maps an engine section name to its short lifecycle-column label.
func phaseLabel(section string) string {
	switch section {
	case "":
		return ""
	case "setup":
		return "setup"
	case "steadyState":
		return "steady"
	case "steadyState:recovery":
		return "recovery"
	case "method":
		return "method"
	case "verify":
		return "verify"
	case "teardown":
		return "teardown"
	default:
		return section
	}
}

// Summary prints the run roll-up after the stream ends: one counts line, then
// one line per non-passing scenario with its reason.
func Summary(w io.Writer, res engine.RunResult, p Palette) {
	var passed, failed, errored, inconclusive, findings int
	for _, sc := range res.Scenarios {
		switch sc.Verdict {
		case engine.ScenarioPassed:
			passed++
		case engine.ScenarioFailed:
			failed++
		case engine.ScenarioErrored:
			errored++
		case engine.ScenarioInconclusive:
			inconclusive++
		}
		for _, f := range sc.Findings {
			if !f.NowPasses {
				findings++
			}
		}
	}

	n := len(res.Scenarios)
	parts := []string{p.Pass(fmt.Sprintf("%d passed", passed))}
	if failed > 0 {
		parts = append(parts, p.Fail(fmt.Sprintf("%d failed", failed)))
	}
	if errored > 0 {
		parts = append(parts, p.Fail(fmt.Sprintf("%d errored", errored)))
	}
	if inconclusive > 0 {
		parts = append(parts, p.Warn(fmt.Sprintf("%d inconclusive", inconclusive)))
	}
	line := fmt.Sprintf("\n%d scenario%s: %s", n, plural(n), strings.Join(parts, ", "))
	if findings > 0 {
		line += " " + p.Dim("—") + " " + p.Finding(fmt.Sprintf("%d finding%s held", findings, plural(findings)))
	}
	if !res.Start.IsZero() && !res.End.IsZero() {
		line += " " + p.Dim("("+res.End.Sub(res.Start).Round(time.Second).String()+")")
	}
	fmt.Fprintln(w, line)

	for _, sc := range res.Scenarios {
		if sc.Verdict == engine.ScenarioPassed {
			continue
		}
		name := sc.Name
		if sc.Suite != "" {
			name = sc.Suite + "/" + sc.Name
		}
		fmt.Fprintf(w, "  %s %s %s %s\n",
			verdictGlyph(p, string(sc.Verdict)), p.Verdict(string(sc.Verdict)), name, p.Dim("— "+sc.Reason))
	}
}

// verdictGlyph is the leading symbol for a verdict, colored to match.
func verdictGlyph(p Palette, v string) string {
	switch v {
	case string(engine.ScenarioPassed):
		return p.Pass("✔")
	case string(engine.ScenarioFailed), string(engine.ScenarioErrored):
		return p.Fail("✘")
	case string(engine.ScenarioInconclusive):
		return p.Warn("◐")
	default:
		return " "
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
