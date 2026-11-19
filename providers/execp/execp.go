// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package execp is the exec built-in provider: the escape hatch.
package execp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

// termGrace is how long a cancelled command's process group gets to handle
// SIGTERM (and clean up) before the group is force-killed. It is shorter than
// cmd.WaitDelay so the graceful path completes before Go's own backstop fires.
const termGrace = 2 * time.Second

type Provider struct {
	dir string
	env []string // resolved project env as KEY=VALUE, layered under the step-level env:
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
	// The resolved project env is layered onto every command so a shelled-out
	// tool sees the declared env: block (sourced from .env/--env-file/OS), the
	// same values the docker provider forwards to compose. Sorted for a
	// deterministic environment; env keys are unique so order is immaterial.
	if m, ok := cfg["projectEnv"].(map[string]any); ok && len(m) > 0 {
		p.env = make([]string, 0, len(m))
		for k, v := range m {
			p.env = append(p.env, fmt.Sprintf("%s=%v", k, v))
		}
		sort.Strings(p.env)
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
			// expectExit makes an exit code the observation instead of a
			// verdict: a diagnostic that exits 3 on the condition under test
			// passes with expectExit: 3, and any other code (including 0)
			// fails the step.
			{Name: "expectExit", Type: "number"},
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
	var killMu sync.Mutex
	var killTimer *time.Timer
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
		killMu.Lock()
		killTimer = time.AfterFunc(termGrace, func() { _ = syscall.Kill(pgid, syscall.SIGKILL) })
		killMu.Unlock()
		return nil
	}
	// Backstop: if a child still holds a pipe after the process exits, do not
	// wait on it indefinitely. Outlasts termGrace so the graceful path wins.
	cmd.WaitDelay = 5 * time.Second
	cmd.Dir = p.dir
	if d, ok := args["dir"].(string); ok && d != "" {
		cmd.Dir = d
	}
	// Precedence (os/exec keeps the last value for a duplicate key): ambient
	// process env, then the project env: block, then the step-level env: arg —
	// most specific wins.
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, p.env...)
	if env, ok := args["env"].(map[string]any); ok {
		for k, v := range env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%v", k, v))
		}
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	// The group exited: an armed SIGKILL backstop must not fire later against
	// a possibly-recycled process group id.
	killMu.Lock()
	if killTimer != nil {
		killTimer.Stop()
	}
	killMu.Unlock()
	combined := stdout.String() + stderr.String()
	// Anything but a command that ran to its own exit (start failure, step
	// timeout or cancellation, death by signal) is an error no expectExit can
	// bless: the command did not run to completion, so its code is not data.
	stderrMsg := strings.TrimSpace(stderr.String())
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return sdk.VerbResult{Output: combined},
			fmt.Errorf("exec.run %q: %w, stderr: %s", cmdStr, err, stderrMsg)
	}
	code := cmd.ProcessState.ExitCode() // -1 when killed by a signal
	meta := map[string]any{"exitCode": code}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return sdk.VerbResult{Output: combined, Meta: meta},
			fmt.Errorf("exec.run %q: interrupted (%w), stderr: %s", cmdStr, ctxErr, stderrMsg)
	}
	if code < 0 {
		return sdk.VerbResult{Output: combined, Meta: meta},
			fmt.Errorf("exec.run %q: %w, stderr: %s", cmdStr, err, stderrMsg)
	}
	if want, ok := conv.ToFloat(args["expectExit"]); ok {
		if want != math.Trunc(want) || want < 0 {
			return sdk.VerbResult{Output: combined, Meta: meta},
				fmt.Errorf("exec.run: expectExit must be a non-negative integer, got %v", args["expectExit"])
		}
		if code != int(want) {
			return sdk.VerbResult{Output: combined, Meta: meta},
				fmt.Errorf("exec.run %q: exit %d, expected %d, stderr: %s", cmdStr, code, int(want), stderrMsg)
		}
	} else if err != nil {
		return sdk.VerbResult{Output: combined, Meta: meta},
			fmt.Errorf("exec.run %q: %w, stderr: %s", cmdStr, err, stderrMsg)
	}
	out := strings.TrimSpace(stdout.String())
	var value any = out
	var decoded any
	if json.Unmarshal([]byte(out), &decoded) == nil && out != "" {
		value = decoded
	}
	return sdk.VerbResult{Value: value, Output: combined, Meta: meta}, nil
}
