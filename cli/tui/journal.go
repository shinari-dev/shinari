// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/shinari-dev/shinari/core/engine"
)

// LoadJournal reads a journal.jsonl file (one JSON engine.Event per line) into
// an event slice for replay.
func LoadJournal(path string) ([]engine.Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []engine.Event
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e engine.Event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
