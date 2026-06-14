// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package execp

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunKillsChildTreeOnCancel(t *testing.T) {
	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		// The backgrounded `sleep` outlives the shell and inherits the
		// stdout pipe. Without killing the whole process group on cancel,
		// cmd.Wait blocks until the sleep exits (30s).
		_, _ = p.Run(ctx, "run", map[string]any{"cmd": "sleep 30 & echo started; wait"})
		close(done)
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("exec.run did not return promptly after cancel; child process group not killed")
	}
}

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
