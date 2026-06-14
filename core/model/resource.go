// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package model holds the typed resource model: every Shinari file is a
// resource recognized by its apiVersion/kind header, never by filename.
package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

const (
	APIVersionV1 = "shinari/v1"

	KindScenario = "Scenario"
	KindProject  = "Project"
	KindProvider = "Provider"
)

// Header is the Kubernetes-style content marker carried by every resource.
type Header struct {
	APIVersion  string `yaml:"apiVersion"`
	Kind        string `yaml:"kind"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Recognized reports whether a parsed document is a Shinari resource:
// a known apiVersion AND a known kind. A file missing either is not ours
// and must be silently ignored by discovery.
func (h Header) Recognized() bool {
	if h.APIVersion != APIVersionV1 {
		return false
	}
	switch h.Kind {
	case KindScenario, KindProject, KindProvider:
		return true
	}
	return false
}

// ParseHeader decodes only the header fields from raw YAML. It returns
// ok=false (and no error) when the document is not a Shinari resource.
func ParseHeader(data []byte) (Header, bool, error) {
	var h Header
	if err := yaml.Unmarshal(data, &h); err != nil {
		// Not even YAML we can read a header from: not a resource.
		return Header{}, false, nil
	}
	if !h.Recognized() {
		return h, false, nil
	}
	if h.Name == "" {
		return h, true, fmt.Errorf("resource kind %s is missing required field 'name'", h.Kind)
	}
	return h, true, nil
}
