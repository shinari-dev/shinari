// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package golden is the findings ledger at the CLI edge: it reads, writes, and
// diffs the acknowledged set of known gaps against a run's observed findings.
// Core never sees it; the golden is an input expectation, not engine memory.
package golden

import (
	"fmt"
	"io"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/shinari-dev/shinari/core/engine"
)

// File is the on-disk golden ledger: the acknowledged set of known gaps.
type File struct {
	Findings []Entry `yaml:"findings"`
}

// Entry is one acknowledged finding, keyed by its stable id.
type Entry struct {
	ID        string `yaml:"id"`
	Scenario  string `yaml:"scenario"`
	Narrative string `yaml:"narrative"`
}

// Drift is the difference between the golden and what a run observed.
type Drift struct {
	Unexpected []engine.FindingRecord // observed, not acknowledged in the golden
	Missing    []Entry                // acknowledged, but no longer observed
}

// Empty reports whether the run matched the golden exactly.
func (d Drift) Empty() bool { return len(d.Unexpected) == 0 && len(d.Missing) == 0 }

// Report writes a human-readable drift summary.
func (d Drift) Report(w io.Writer) error {
	for _, f := range d.Unexpected {
		if _, err := fmt.Fprintf(w, "  + new finding not in ledger: %s [%s] %q (run -u to acknowledge)\n",
			f.Scenario, f.ID, f.Narrative); err != nil {
			return err
		}
	}
	for _, e := range d.Missing {
		if _, err := fmt.Fprintf(w, "  - ledger finding no longer observed: %s [%s] %q (gap fixed? run -u to drop it)\n",
			e.Scenario, e.ID, e.Narrative); err != nil {
			return err
		}
	}
	return nil
}

// Reconcile diffs the observed findings of a run against the golden, by id. A
// finding that now passes (NowPasses) is excluded: core already flips the run
// for that, and it is not an acknowledged active gap.
func Reconcile(g File, observed []engine.FindingRecord) Drift {
	want := map[string]Entry{}
	for _, e := range g.Findings {
		want[e.ID] = e
	}
	seen := map[string]bool{}
	var d Drift
	for _, f := range observed {
		if f.NowPasses {
			continue
		}
		seen[f.ID] = true
		if _, ok := want[f.ID]; !ok {
			d.Unexpected = append(d.Unexpected, f)
		}
	}
	for id, e := range want {
		if !seen[id] {
			d.Missing = append(d.Missing, e)
		}
	}
	sort.Slice(d.Missing, func(i, j int) bool { return d.Missing[i].ID < d.Missing[j].ID })
	return d
}

// FromObserved builds a golden File from a run's observed active findings,
// sorted by id for a stable, diffable file.
func FromObserved(observed []engine.FindingRecord) File {
	var f File
	for _, r := range observed {
		if r.NowPasses {
			continue
		}
		f.Findings = append(f.Findings, Entry{ID: r.ID, Scenario: r.Scenario, Narrative: r.Narrative})
	}
	sort.Slice(f.Findings, func(i, j int) bool { return f.Findings[i].ID < f.Findings[j].ID })
	return f
}

// Load reads a golden file. A missing file is not an error: it returns an empty
// File and exists=false so the caller preserves today's no-ledger behavior.
func Load(path string) (File, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return File{}, false, nil
	}
	if err != nil {
		return File{}, false, err
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return File{}, true, err
	}
	return f, true, nil
}

// Write marshals the golden file to path with a header comment.
func Write(path string, f File) error {
	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	header := "# shinari findings ledger — acknowledged known gaps.\n# Regenerate with `shinari run -u`. Do not hand-edit ids.\n"
	return os.WriteFile(path, append([]byte(header), data...), 0o644)
}
