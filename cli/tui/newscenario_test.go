// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"
	"testing"
)

func TestNewModelViewHasFields(t *testing.T) {
	m := newNewModel()
	out := m.View()
	for _, want := range []string{"name", "suite", "minimal", "fault-inject"} {
		if !strings.Contains(out, want) {
			t.Fatalf("new-scenario form missing %q in:\n%s", want, out)
		}
	}
}

func TestNewModelSubmitWritesFile(t *testing.T) {
	root := t.TempDir()
	m := newNewModel()
	m.name.SetValue("demo")
	m.suite.SetValue("s")
	cmd := m.submit(root)
	msg := cmd()
	cm, ok := msg.(createdMsg)
	if !ok || cm.err != nil {
		t.Fatalf("submit failed: %#v", msg)
	}
	if !strings.HasSuffix(cm.path, "s/demo.yml") {
		t.Fatalf("unexpected path %s", cm.path)
	}
}
