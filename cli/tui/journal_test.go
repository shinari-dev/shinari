// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shinari-dev/shinari/core/engine"
)

func TestLoadJournalParsesEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	content := `{"type":"scenario.started","time":"2026-06-25T10:00:00Z","scenario":"s"}
{"type":"finding.recorded","time":"2026-06-25T10:00:01Z","scenario":"s","step":"chk","payload":{"id":"sha-x","narrative":"gap"}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	events, err := LoadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != engine.EvScenarioStarted {
		t.Fatalf("first event type: %q", events[0].Type)
	}
}
