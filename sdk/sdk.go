// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package sdk defines the narrow contract between the Shinari engine and
// providers. Provider plugins link only this package — never the engine.
package sdk

import "context"

// Kind classifies a verb: actions mutate the system under test, probes
// observe it, assertions judge it. Kind drives dry-run (skip actions),
// steadyState re-runs (probes only) and the verdict split.
type Kind string

const (
	KindAction    Kind = "action"
	KindProbe     Kind = "probe"
	KindAssertion Kind = "assertion"
)

// Effect classifies the fault a verb injects, orthogonal to Kind. It is the
// declarative replacement for "is this a fault verb?" so the engine (fault
// tracking) and validate (the recovery heuristic) read it from the spec
// instead of matching hardcoded verb names — a third-party fault verb just
// declares its Effect. Most verbs (workload, setup, probes, assertions) are
// EffectNone; only deliberate faults set one.
type Effect string

const (
	EffectNone        Effect = ""            // not a fault
	EffectOutage      Effect = "outage"      // drops or blocks work: kill, partition, packet loss, NXDOMAIN…
	EffectDegradation Effect = "degradation" // slows but does not drop work: added latency, throttled bandwidth
)

// ArgSpec describes one argument of a verb — enough for validate,
// deliberately not a type system.
type ArgSpec struct {
	Name     string
	Type     string // "string" | "number" | "bool" | "list" | "map" | "any"
	Required bool
}

// VerbSpec declares one capability of a provider. Name is local
// (snake_case); the engine addresses it as <instance>.<name>.
type VerbSpec struct {
	Name        string
	Kind        Kind
	SideEffects bool
	// Effect declares the fault this verb injects (EffectNone for non-faults).
	// A fault always has SideEffects; the converse does not hold — workload
	// and setup verbs mutate without being faults.
	Effect Effect
	Args   []ArgSpec
	// Primary names the arg bound when `with:` is a scalar or list
	// shorthand instead of a map.
	Primary string
}

// VerbResult is what a verb returns: a structured value (JSON-decoded when
// applicable) plus the raw textual output for logs/diagnostics.
type VerbResult struct {
	Value  any
	Output string
}

// Provider is the whole contract. Configure is called once per configured
// instance; Run is called per step with args already validated against the
// verb's ArgSpecs.
type Provider interface {
	Type() string
	Configure(cfg map[string]any) error
	Verbs() []VerbSpec
	Run(ctx context.Context, verb string, args map[string]any) (VerbResult, error)
}
