// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package render

import "github.com/shinari-dev/shinari/core/engine"

// Palette colors console output. The zero value is plain: every method
// returns its argument untouched, so a Console or Summary built without a
// palette (as the tests do) prints exactly as before. NewPalette(true) turns
// on ANSI; the CLI decides that from --color, NO_COLOR, and TTY detection.
//
// Findings and faults use the ember 256-color tones (202/208) that match the
// docs-site brand; pass/fail/verdict use the conventional green/red/yellow.
type Palette struct {
	on bool
}

// NewPalette returns a palette that emits ANSI when enabled is true.
func NewPalette(enabled bool) Palette { return Palette{on: enabled} }

func (p Palette) wrap(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p Palette) Pass(s string) string    { return p.wrap("32", s) }       // green
func (p Palette) Fail(s string) string    { return p.wrap("31;1", s) }     // bold red
func (p Palette) Finding(s string) string { return p.wrap("38;5;202", s) } // ember
func (p Palette) Fault(s string) string   { return p.wrap("38;5;208", s) } // amber
func (p Palette) Gate(s string) string    { return p.wrap("36", s) }       // cyan
func (p Palette) Skip(s string) string    { return p.wrap("2", s) }        // dim
func (p Palette) Dim(s string) string     { return p.wrap("2", s) }        // dim
func (p Palette) Bold(s string) string    { return p.wrap("1", s) }
func (p Palette) Warn(s string) string    { return p.wrap("33", s) } // yellow

// Verdict colors a scenario or run verdict by its outcome.
func (p Palette) Verdict(v string) string {
	switch v {
	case string(engine.ScenarioPassed):
		return p.Pass(v)
	case string(engine.ScenarioFailed), string(engine.ScenarioErrored):
		return p.Fail(v)
	case string(engine.ScenarioInconclusive):
		return p.Warn(v)
	default:
		return v
	}
}
