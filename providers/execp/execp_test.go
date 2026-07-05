// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package execp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestRunGracefulCancelLetsChildCleanUp(t *testing.T) {
	// A backgrounded fault tool (e.g. pumba) reverts its kernel-level rule in a
	// SIGTERM handler. Cancel must give the process group a chance to run that
	// handler before escalating to SIGKILL, or the fault is never cleared.
	p := New()
	marker := filepath.Join(t.TempDir(), "cleaned")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_, _ = p.Run(ctx, "run", map[string]any{
			"cmd": fmt.Sprintf(`trap 'echo done > %s; exit 0' TERM; echo ready; sleep 30`, marker),
		})
		close(done)
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("exec.run did not return promptly after cancel")
	}
	data, err := os.ReadFile(marker)
	if err != nil || !strings.Contains(string(data), "done") {
		t.Fatalf("child did not run its SIGTERM cleanup before exit; marker=%q err=%v", string(data), err)
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

// The resolved project env: block reaches a shelled-out command, the same
// values the docker provider forwards to compose.
func TestRunProjectEnv(t *testing.T) {
	p := New()
	if err := p.Configure(map[string]any{
		"projectEnv": map[string]any{"REGION": "us-east-1"},
	}); err != nil {
		t.Fatal(err)
	}
	res, err := p.Run(context.Background(), "run", map[string]any{"cmd": "echo $REGION"})
	if err != nil || res.Value != "us-east-1" {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

// Precedence: a step-level env: arg overrides the project env: block, which in
// turn overrides an ambient OS var — most specific wins.
func TestRunEnvPrecedence(t *testing.T) {
	t.Setenv("REGION", "ambient")
	p := New()
	if err := p.Configure(map[string]any{
		"projectEnv": map[string]any{"REGION": "project"},
	}); err != nil {
		t.Fatal(err)
	}
	// Project env beats the ambient value...
	res, err := p.Run(context.Background(), "run", map[string]any{"cmd": "echo $REGION"})
	if err != nil || res.Value != "project" {
		t.Fatalf("project env should beat ambient: res=%v err=%v", res, err)
	}
	// ...and a step-level env: arg beats the project env.
	res, err = p.Run(context.Background(), "run", map[string]any{
		"cmd": "echo $REGION", "env": map[string]any{"REGION": "step"},
	})
	if err != nil || res.Value != "step" {
		t.Fatalf("step env should beat project env: res=%v err=%v", res, err)
	}
}
