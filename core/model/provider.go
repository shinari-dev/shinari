// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComposedVerb is one macro of a kind: Provider resource: either a
// do: list of steps or a single probe: step.
type ComposedVerb struct {
	Params []string `yaml:"params"`
	Do     []Step   `yaml:"do"`
	Probe  *Step    `yaml:"probe"`
}

// ParamNames returns parameter names with the optional marker stripped,
// plus which are optional ("inputs?" → "inputs", optional).
func (v ComposedVerb) ParamNames() (names []string, optional map[string]bool) {
	optional = map[string]bool{}
	for _, p := range v.Params {
		if strings.HasSuffix(p, "?") {
			name := strings.TrimSuffix(p, "?")
			names = append(names, name)
			optional[name] = true
		} else {
			names = append(names, p)
		}
	}
	return names, optional
}

// ProviderDef is the kind: Provider resource — a composed provider:
// domain vocabulary as YAML macros over other verbs, zero Go.
type ProviderDef struct {
	Header `yaml:",inline"`
	Verbs  map[string]ComposedVerb `yaml:"verbs"`

	File string `yaml:"-"`
}

func ParseProviderDef(data []byte, file string) (*ProviderDef, error) {
	var p ProviderDef
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("%s: malformed Provider: %w", file, err)
	}
	for name, v := range p.Verbs {
		if len(v.Do) == 0 && v.Probe == nil {
			return nil, fmt.Errorf("%s: provider %s verb %s: needs a 'do' list or a 'probe'", file, p.Name, name)
		}
		if len(v.Do) > 0 && v.Probe != nil {
			return nil, fmt.Errorf("%s: provider %s verb %s: 'do' and 'probe' are mutually exclusive", file, p.Name, name)
		}
	}
	p.File = file
	return &p, nil
}
