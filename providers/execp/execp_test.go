// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package execp

import (
	"context"
	"strings"
	"testing"
)

func TestRunCapturesStdout(t *testing.T) {
	p := New()
	res, err := p.Run(context.Background(), "run", map[string]any{"cmd": "echo hello"})
	if err != nil || res.Value != "hello" {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestRunDecodesJSON(t *testing.T) {
	p := New()
	res, err := p.Run(context.Background(), "run", map[string]any{"cmd": `echo '{"n": 3}'`})
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := res.Value.(map[string]any); !ok || m["n"] != float64(3) {
		t.Fatalf("value = %v (%T)", res.Value, res.Value)
	}
}

func TestRunFailureNamesStderr(t *testing.T) {
	p := New()
	_, err := p.Run(context.Background(), "run", map[string]any{"cmd": "echo nope >&2; exit 3"})
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("want stderr in error, got %v", err)
	}
}

func TestRunEnv(t *testing.T) {
	p := New()
	res, err := p.Run(context.Background(), "run", map[string]any{
		"cmd": "echo $GREETING", "env": map[string]any{"GREETING": "yo"},
	})
	if err != nil || res.Value != "yo" {
		t.Fatalf("res=%v err=%v", res, err)
	}
}
