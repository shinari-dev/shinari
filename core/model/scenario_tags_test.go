// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package model

import "testing"

func TestParseScenarioTags(t *testing.T) {
	data := []byte("apiVersion: shinari/v1\nkind: Scenario\nname: t\ntags: [slow, redis]\n")
	sc, err := ParseScenario(data, "t.yml")
	if err != nil {
		t.Fatalf("ParseScenario: %v", err)
	}
	if len(sc.Tags) != 2 || sc.Tags[0] != "slow" || sc.Tags[1] != "redis" {
		t.Fatalf("Tags = %v, want [slow redis]", sc.Tags)
	}
}

func TestParseScenarioNoTags(t *testing.T) {
	data := []byte("apiVersion: shinari/v1\nkind: Scenario\nname: t\n")
	sc, err := ParseScenario(data, "t.yml")
	if err != nil {
		t.Fatalf("ParseScenario: %v", err)
	}
	if len(sc.Tags) != 0 {
		t.Fatalf("Tags = %v, want empty", sc.Tags)
	}
}
