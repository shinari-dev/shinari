// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/shinari-dev/shinari/core/model"
)

// FindingID is the stable identity of a finding step: its explicit id: when
// set, else a derived fingerprint over the structural check (scenario,
// section, verb, and authored with:). The narrative and desc are excluded so
// they can be reworded without breaking golden matching or history.
func FindingID(scenario, section string, st *model.Step) string {
	if st.ID != "" {
		return st.ID
	}
	input := strings.Join([]string{scenario, section, st.Run, canonicalWith(st.With)}, "\x00")
	sum := sha256.Sum256([]byte(input))
	return "sha-" + hex.EncodeToString(sum[:])[:12]
}

// canonicalWith renders a step's with: node to a deterministic string. yaml.v3
// marshals map keys in sorted order, so the result is independent of the
// source key order. A zero node (no with:) renders to the empty string.
func canonicalWith(n yaml.Node) string {
	if n.Kind == 0 {
		return ""
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return ""
	}
	out, err := yaml.Marshal(v)
	if err != nil {
		return ""
	}
	return string(out)
}
