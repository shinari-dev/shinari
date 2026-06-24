// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package execp is the exec built-in provider: the escape hatch.
package execp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/shinari-dev/shinari/sdk"
)

// termGrace is how long a cancelled command's process group gets to handle
// SIGTERM (and clean up) before the group is force-killed. It is shorter than
// cmd.WaitDelay so the graceful path completes before Go's own backstop fires.
const termGrace = 2 * time.Second

type Provider struct {
	dir string
}

func init() { sdk.Register("exec", New) }

func New() sdk.Provider { return &Provider{} }

func (p *Provider) Type() string { return "exec" }

func (p *Provider) Configure(cfg map[string]any) error {
	if d, ok := cfg["projectDir"].(string); ok {
		p.dir = d // default: commands run from the project root
	}
	if d, ok := cfg["dir"].(string); ok {
		p.dir = d
	}
	return nil
}

func (p *Provider) Verbs() []sdk.VerbSpec {
	return []sdk.VerbSpec{{
		Name:        "run",
		Kind:        sdk.KindAction, // default; the one step-level kind override
		SideEffects: true,
		Primary:     "cmd",
		Args: []sdk.ArgSpec{
			{Name: "cmd", Type: "string", Required: true},
			{Name: "env", Type: "map"},
			{Name: "dir", Type: "string"},
		},
	}}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	if verb != "run" {
		return sdk.VerbResult{}, fmt.Errorf("exec has no verb %q", verb)
	}
	cmdStr, _ := args["cmd"].(string)
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	// Run in its own process group so cancellation kills the whole tree, not
	// just the shell. A backgrounded command (via the `background` builtin)
	// often spawns children (e.g. a fault tool's helper) that would otherwise
	// outlive the shell, inherit its stdout pipe, and block cmd.Wait forever.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		pgid := -cmd.Process.Pid
		// Graceful first: a backgrounded fault tool (e.g. pumba) reverts its
		// kernel-level rule in a SIGTERM handler, so a hard kill would leave the
		// system faulted. Give the whole group that chance, then escalate to
		// SIGKILL if it ignores the term within the grace window.
		_ = syscall.Kill(pgid, syscall.SIGTERM)
		time.AfterFunc(termGrace, func() { _ = syscall.Kill(pgid, syscall.SIGKILL) })
		return nil
	}
	// Backstop: if a child still holds a pipe after the process exits, do not
	// wait on it indefinitely. Outlasts termGrace so the graceful path wins.
	cmd.WaitDelay = 5 * time.Second
	cmd.Dir = p.dir
	if d, ok := args["dir"].(string); ok && d != "" {
		cmd.Dir = d
	}
	cmd.Env = os.Environ()
	if env, ok := args["env"].(map[string]any); ok {
		for k, v := range env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%v", k, v))
		}
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	combined := stdout.String() + stderr.String()
	if err != nil {
		return sdk.VerbResult{Output: combined},
			fmt.Errorf("exec.run %q: %w — stderr: %s", cmdStr, err, strings.TrimSpace(stderr.String()))
	}
	out := strings.TrimSpace(stdout.String())
	var value any = out
	var decoded any
	if json.Unmarshal([]byte(out), &decoded) == nil && out != "" {
		value = decoded
	}
	return sdk.VerbResult{Value: value, Output: combined}, nil
}
