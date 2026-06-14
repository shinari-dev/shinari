// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/shinari-dev/shinari/sdk"
)

// Step is the single step shape: the verb is a value, never a key.
// With stays a yaml.Node so scalar/list/map shorthands survive until the
// registry binds them against the verb's arg spec.
type Step struct {
	Run        string            `yaml:"run"`
	With       yaml.Node         `yaml:"with"`
	As         string            `yaml:"as"`
	Read       string            `yaml:"read"` // jq transform of the result value, applied before as:/capture:
	Capture    map[string]string `yaml:"capture"`
	Desc       string            `yaml:"desc"`
	OnAbsent   string            `yaml:"onAbsent"`
	SkipReason string            `yaml:"skipReason"`
	Finding    string            `yaml:"finding"`
	Kind       string            `yaml:"kind"`   // override the verb's kind (the exec.run escape hatch)
	Effect     string            `yaml:"effect"` // declare the fault a polymorphic verb (exec.run, http.post) injects

	// wait_until extras (builtin envelope): the
	// nested probe plus assert-operator keys live beside `with` in examples
	// like { run: wait_until, with: { probe: {...}, matches: ..., timeout: ... } },
	// so they arrive through With; nothing extra here.

	// File/line for error messages, filled by the loader.
	File string `yaml:"-"`
	Line int    `yaml:"-"`
}

// reservedStepKeys are the only keys allowed in a step envelope.
var reservedStepKeys = map[string]bool{
	"run": true, "with": true, "as": true, "read": true, "capture": true,
	"desc": true, "onAbsent": true, "skipReason": true, "finding": true, "kind": true,
	"effect": true,
}

// UnmarshalYAML enforces the closed envelope: one run:, reserved keys only.
func (s *Step) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a step must be a mapping with a 'run' key", node.Line)
	}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if !reservedStepKeys[key] {
			return fmt.Errorf("line %d: unknown step key %q (reserved envelope keys: run, with, as, read, capture, desc, onAbsent, skipReason, finding, kind, effect)", node.Content[i].Line, key)
		}
	}
	type plain Step
	var p plain
	if err := node.Decode(&p); err != nil {
		return err
	}
	*s = Step(p)
	s.Line = node.Line
	if s.Run == "" {
		return fmt.Errorf("line %d: step is missing required key 'run'", node.Line)
	}
	switch sdk.Effect(s.Effect) {
	case sdk.EffectNone, sdk.EffectOutage, sdk.EffectDegradation:
	default:
		return fmt.Errorf("line %d: invalid effect %q (one of: outage, degradation)", node.Line, s.Effect)
	}
	return nil
}

// DecodeWith decodes the step's with: node into a plain Go value (map,
// list or scalar), or nil when with: is absent. Interpolation is the
// caller's concern — this is the raw shape the registry and validator bind
// against.
func (s *Step) DecodeWith() (any, error) {
	if s.With.IsZero() {
		return nil, nil
	}
	var raw any
	if err := s.With.Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// Phase is one ordered list of steps under method:.
type Phase struct {
	Phase string `yaml:"phase"`
	Steps []Step `yaml:"steps"`
}

// Scenario is the kind: Scenario resource.
type Scenario struct {
	Header      `yaml:",inline"`
	Providers   map[string]ProviderConfig `yaml:"providers"`
	Vars        map[string]any            `yaml:"vars"`
	Setup       []Step                    `yaml:"setup"`
	SteadyState []Step                    `yaml:"steadyState"`
	Method      []Phase                   `yaml:"method"`
	Verify      []Step                    `yaml:"verify"`
	Teardown    []Step                    `yaml:"teardown"`
	// HasTeardown distinguishes "absent" (default [docker.down] applies)
	// from "explicitly empty".
	HasTeardown bool `yaml:"-"`

	File  string `yaml:"-"`
	Suite string `yaml:"-"`
}

// ParseScenario decodes a full scenario body. The header is assumed
// recognized; a malformed body here is an error, not a skip.
func ParseScenario(data []byte, file string) (*Scenario, error) {
	var sc Scenario
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("%s: malformed Scenario: %w", file, err)
	}
	var raw map[string]any
	_ = yaml.Unmarshal(data, &raw)
	_, sc.HasTeardown = raw["teardown"]
	sc.File = file
	for si := range sc.Setup {
		sc.Setup[si].File = file
	}
	for si := range sc.SteadyState {
		sc.SteadyState[si].File = file
	}
	for pi := range sc.Method {
		for si := range sc.Method[pi].Steps {
			sc.Method[pi].Steps[si].File = file
		}
	}
	for si := range sc.Verify {
		sc.Verify[si].File = file
	}
	for si := range sc.Teardown {
		sc.Teardown[si].File = file
	}
	return &sc, nil
}

// Section is a named step list of a scenario.
type Section struct {
	Name  string
	Steps []Step
}

// Sections returns the step lists in execution order, for callers
// (validate, executor) that iterate uniformly.
func (sc *Scenario) Sections() []Section {
	out := []Section{
		{"setup", sc.Setup},
		{"steadyState", sc.SteadyState},
	}
	for _, ph := range sc.Method {
		out = append(out, Section{"method:" + ph.Phase, ph.Steps})
	}
	out = append(out, Section{"verify", sc.Verify})
	out = append(out, Section{"teardown", sc.Teardown})
	return out
}
