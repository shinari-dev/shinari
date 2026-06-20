// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ProviderConfig is one entry of a providers: block. The map key is
// the instance name (= verb namespace). Source/Version are optional and
// only used once the install path exists; Use points at a local
// composed provider.
type ProviderConfig struct {
	Use     string         `yaml:"use"`
	Source  string         `yaml:"source"`
	Version string         `yaml:"version"`
	Config  map[string]any `yaml:"config"`
}

// Project is the kind: Project resource — the root config.
type Project struct {
	Header    `yaml:",inline"`
	Providers map[string]ProviderConfig `yaml:"providers"`
	Vars      map[string]any            `yaml:"vars"`
	Env       map[string]any            `yaml:"env"`

	File string `yaml:"-"`
	Dir  string `yaml:"-"`
}

func ParseProject(data []byte, file string) (*Project, error) {
	var p Project
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("%s: malformed Project: %w", file, err)
	}
	p.File = file
	return &p, nil
}

// MergeProviders overlays a scenario's providers block on the project's
// (later wins, per-key; config maps merge shallowly).
func MergeProviders(base, over map[string]ProviderConfig) map[string]ProviderConfig {
	out := make(map[string]ProviderConfig, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		if prev, ok := out[k]; ok {
			merged := prev
			if v.Use != "" {
				merged.Use = v.Use
			}
			if v.Source != "" {
				merged.Source = v.Source
			}
			if v.Version != "" {
				merged.Version = v.Version
			}
			if len(v.Config) > 0 {
				cfg := make(map[string]any, len(prev.Config)+len(v.Config))
				for ck, cv := range prev.Config {
					cfg[ck] = cv
				}
				for ck, cv := range v.Config {
					cfg[ck] = cv
				}
				merged.Config = cfg
			}
			out[k] = merged
		} else {
			out[k] = v
		}
	}
	return out
}
