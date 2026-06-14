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

	"github.com/shinari-dev/shinari/sdk"
)

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
			fmt.Errorf("exec.run %q: %v — stderr: %s", cmdStr, err, strings.TrimSpace(stderr.String()))
	}
	out := strings.TrimSpace(stdout.String())
	var value any = out
	var decoded any
	if json.Unmarshal([]byte(out), &decoded) == nil && out != "" {
		value = decoded
	}
	return sdk.VerbResult{Value: value, Output: combined}, nil
}
