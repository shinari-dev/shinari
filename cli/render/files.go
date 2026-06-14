// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/shinari-dev/shinari/core/engine"
)

// TSV writes results.tsv: one row per check.
func TSV(w io.Writer, res engine.RunResult) error {
	if _, err := fmt.Fprintln(w, "scenario\tsection\tcheck\tverdict\tdurationMs\terror"); err != nil {
		return err
	}
	for _, sc := range res.Scenarios {
		for _, st := range sc.Steps {
			label := st.Label()
			ms := st.End.Sub(st.Start).Milliseconds()
			errMsg := strings.ReplaceAll(st.Err, "\t", " ")
			errMsg = strings.ReplaceAll(errMsg, "\n", " ")
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
				sc.Name, st.Section, label, st.Verdict, ms, errMsg); err != nil {
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
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
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
