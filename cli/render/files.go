// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/shinari-dev/shinari/core/engine"
)

// tsvCell strips the characters that would break TSV row structure.
var tsvCell = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// TSV writes results.tsv: one row per check. Every author-controlled cell is
// escaped — a tab or newline in a scenario name or desc must not split a row.
func TSV(w io.Writer, res engine.RunResult) error {
	if _, err := fmt.Fprintln(w, "scenario\tsection\tcheck\tverdict\tdurationMs\terror"); err != nil {
		return err
	}
	for _, sc := range res.Scenarios {
		for _, st := range sc.Steps {
			ms := st.End.Sub(st.Start).Milliseconds()
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
				tsvCell.Replace(sc.Name), tsvCell.Replace(st.Section), tsvCell.Replace(st.Label()),
				st.Verdict, ms, tsvCell.Replace(st.Err)); err != nil {
				return err
			}
		}
	}
	return nil
}

// resultsDoc is the results.json shape: the full RunResult plus the
// roll-up verdict, so CI consumes one file.
type resultsDoc struct {
	Verdict   engine.ScenarioVerdict  `json:"verdict"`
	ExitCode  int                     `json:"exitCode"`
	Scenarios []engine.ScenarioResult `json:"scenarios"`
}

// ResultsJSON writes results.json — the CI artifact.
func ResultsJSON(w io.Writer, res engine.RunResult) error {
	for i := range res.Scenarios {
		for j := range res.Scenarios[i].Steps {
			finiteJSON(res.Scenarios[i].Steps[j].Captured)
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resultsDoc{
		Verdict:   res.Verdict(),
		ExitCode:  res.Verdict().ExitCode(),
		Scenarios: res.Scenarios,
	})
}

// Journal writes the serialized event stream, one JSON object per line —
// the journal IS the event stream.
func Journal(w io.Writer, events []engine.Event) error {
	enc := json.NewEncoder(w)
	for _, e := range events {
		finiteJSON(e.Payload)
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
}

// finiteJSON replaces NaN/Inf floats in place: legal jq results, illegal JSON.
// Without this one odd captured value aborts report writing and masks the
// run's verdict behind exit 2.
func finiteJSON(m map[string]any) {
	for k, v := range m {
		m[k] = finiteValue(v)
	}
}

func finiteValue(v any) any {
	switch t := v.(type) {
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return fmt.Sprintf("%v", t)
		}
		return t
	case map[string]any:
		finiteJSON(t)
		return t
	case []any:
		for i, e := range t {
			t[i] = finiteValue(e)
		}
		return t
	default:
		return v
	}
}

// JUnit XML — minimal schema CI servers consume.
type junitTestsuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []junitTestsuite `xml:"testsuite"`
}

type junitTestsuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Errors   int             `xml:"errors,attr"`
	Skipped  int             `xml:"skipped,attr"`
	Time     string          `xml:"time,attr"`
	Cases    []junitTestcase `xml:"testcase"`
}

type junitTestcase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitMessage `xml:"failure,omitempty"`
	Error     *junitMessage `xml:"error,omitempty"`
	Skipped   *junitMessage `xml:"skipped,omitempty"`
	SystemOut string        `xml:"system-out,omitempty"`
}

type junitMessage struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

// JUnit writes junit.xml: one testsuite per scenario; a FINDING renders as
// a pass with the narrative in system-out (it keeps CI green).
func JUnit(w io.Writer, res engine.RunResult) error {
	var doc junitTestsuites
	for _, sc := range res.Scenarios {
		suite := junitTestsuite{
			Name: sc.Name,
			Time: fmt.Sprintf("%.3f", sc.End.Sub(sc.Start).Seconds()),
		}
		for _, st := range sc.Steps {
			label := st.Label()
			tc := junitTestcase{
				Name:      fmt.Sprintf("[%s] %s", st.Section, label),
				Classname: sc.Name,
				Time:      fmt.Sprintf("%.3f", st.End.Sub(st.Start).Seconds()),
			}
			suite.Tests++
			switch st.Verdict {
			case engine.CheckFail:
				if sc.Verdict == engine.ScenarioErrored && st.Section == "setup" {
					suite.Errors++
					tc.Error = &junitMessage{Message: st.Err}
				} else {
					suite.Failures++
					tc.Failure = &junitMessage{Message: st.Err}
				}
			case engine.CheckSkip:
				suite.Skipped++
				tc.Skipped = &junitMessage{Message: st.SkipReason}
			case engine.CheckFinding:
				tc.SystemOut = "FINDING (expected failure, ledger-tracked): " + st.Finding
			}
			suite.Cases = append(suite.Cases, tc)
		}
		if sc.Verdict == engine.ScenarioInconclusive {
			suite.Skipped++
			suite.Tests++
			suite.Cases = append(suite.Cases, junitTestcase{
				Name: "[steadyState] gate", Classname: sc.Name,
				Skipped: &junitMessage{Message: "INCONCLUSIVE: " + sc.Reason},
			})
		}
		doc.Suites = append(doc.Suites, suite)
	}
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// FindingsReport writes the findings report (markdown): per scenario what
// was injected, what held, and what gapped — the ledger rendering.
func FindingsReport(w io.Writer, res engine.RunResult) error {
	fmt.Fprintf(w, "# Findings report\n")
	for _, sc := range res.Scenarios {
		fmt.Fprintf(w, "\n## %s — %s\n", sc.Name, sc.Verdict)
		if sc.Description != "" {
			fmt.Fprintf(w, "\n%s\n", sc.Description)
		}
		if len(sc.Injected) > 0 {
			fmt.Fprintf(w, "\n**Injected**\n")
			for _, in := range sc.Injected {
				fmt.Fprintf(w, "- %s\n", in)
			}
		}
		if len(sc.Held) > 0 {
			fmt.Fprintf(w, "\n**Held**\n")
			for _, h := range sc.Held {
				fmt.Fprintf(w, "- %s\n", h)
			}
		}
		gapped := false
		for _, f := range sc.Findings {
			if !gapped {
				fmt.Fprintf(w, "\n**Gapped**\n")
				gapped = true
			}
			if f.NowPasses {
				fmt.Fprintf(w, "- ~~%s~~ — **now passes**: promote to a hard assertion (check: %s)\n", f.Narrative, f.Check)
			} else {
				fmt.Fprintf(w, "- %s (check: %s)\n  - observed: %s\n", f.Narrative, f.Check, f.Detail)
			}
		}
	}
	return nil
}
