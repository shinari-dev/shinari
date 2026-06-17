// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package validate

import (
	"testing"

	"github.com/shinari-dev/shinari/core/model"
)

func TestValidateTagsBadChar(t *testing.T) {
	sc := &model.Scenario{Tags: []string{"ok", "bad tag", "no&good"}}
	sc.Name = "s"
	sc.File = "s.yml"
	findings := validateTags(sc)
	errs := 0
	for _, f := range findings {
		if f.Rule == 14 && f.Severity == Error {
			errs++
		}
	}
	if errs != 2 {
		t.Fatalf("got %d error findings, want 2: %v", errs, findings)
	}
}

func TestValidateTagsDuplicate(t *testing.T) {
	sc := &model.Scenario{Tags: []string{"slow", "slow"}}
	sc.Name = "s"
	sc.File = "s.yml"
	findings := validateTags(sc)
	warns := 0
	for _, f := range findings {
		if f.Rule == 14 && f.Severity == Warn {
			warns++
		}
	}
	if warns != 1 {
		t.Fatalf("got %d warn findings, want 1: %v", warns, findings)
	}
}

func TestValidateTagsClean(t *testing.T) {
	sc := &model.Scenario{Tags: []string{"slow", "net.core/db-1"}}
	sc.Name = "s"
	sc.File = "s.yml"
	if f := validateTags(sc); len(f) != 0 {
		t.Fatalf("clean tags produced findings: %v", f)
	}
}
