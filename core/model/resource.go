// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package model holds the typed resource model: every Shinari file is a
// resource recognized by its apiVersion/kind header, never by filename.
package model

import (
	"fmt"
	"strings"

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
// A document that carries the apiVersion marker but is not parseable YAML
// is ours-but-broken: ok=true with the parse error, never a silent skip.
func ParseHeader(data []byte) (Header, bool, error) {
	var h Header
	if err := yaml.Unmarshal(data, &h); err != nil {
		if carriesMarker(data) {
			return Header{}, true, fmt.Errorf("malformed YAML in a file marked apiVersion: %s: %w", APIVersionV1, err)
		}
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

// carriesMarker line-scans raw bytes for the apiVersion marker, so a syntax
// error elsewhere in the file cannot demote a Shinari resource to "not ours".
func carriesMarker(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "apiVersion:") && strings.Contains(trimmed, APIVersionV1) {
			return true
		}
	}
	return false
}
