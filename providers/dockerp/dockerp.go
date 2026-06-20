// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package dockerp is the docker built-in provider: lifecycle + process
// faults via the docker compose CLI. The CLI is the stable compose
// contract; there is no first-party compose Go SDK.
package dockerp

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

type Provider struct {
	composeFiles []string
	project      string
	bin          string // overridable for tests
	binArgs      []string
}

func init() { sdk.Register("docker", New) }

func New() sdk.Provider { return &Provider{bin: "docker", binArgs: []string{"compose"}} }

func (p *Provider) Type() string { return "docker" }

func (p *Provider) Configure(cfg map[string]any) error {
	projectDir, _ := cfg["projectDir"].(string)
	if files, ok := cfg["composeFiles"].([]any); ok {
		for _, f := range files {
			path := fmt.Sprintf("%v", f)
			if projectDir != "" && !filepath.IsAbs(path) {
				path = filepath.Join(projectDir, path)
			}
			p.composeFiles = append(p.composeFiles, path)
		}
	}
	if proj, ok := cfg["project"].(string); ok {
		p.project = proj
	}
	// test seam: bin: ["/path/to/stub"] replaces the docker compose binary
	if bin, ok := cfg["bin"].(string); ok && bin != "" {
		p.bin = bin
		p.binArgs = nil
	}
	return nil
}

func (p *Provider) Verbs() []sdk.VerbSpec {
	upArgs := []sdk.ArgSpec{{Name: "services", Type: "list"}, {Name: "wait", Type: "bool"}}
	service := []sdk.ArgSpec{{Name: "service", Type: "string", Required: true}}
	return []sdk.VerbSpec{
		{Name: "up", Kind: sdk.KindAction, SideEffects: true, Primary: "services", Args: upArgs},
		{Name: "down", Kind: sdk.KindAction, SideEffects: true},
		{Name: "kill", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "service", Args: service},
		{Name: "stop", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "service", Args: service},
		{Name: "start", Kind: sdk.KindAction, SideEffects: true, Primary: "service", Args: service},
		{Name: "pause", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "service", Args: service},
		{Name: "unpause", Kind: sdk.KindAction, SideEffects: true, Primary: "service", Args: service},
		{Name: "logs", Kind: sdk.KindProbe, Primary: "service", Args: []sdk.ArgSpec{
			{Name: "service", Type: "string", Required: true},
			{Name: "tail", Type: "string"},
			{Name: "since", Type: "string"},
		}},
	}
}

func (p *Provider) compose(ctx context.Context, args ...string) (string, error) {
	full := append([]string{}, p.binArgs...)
	for _, f := range p.composeFiles {
		full = append(full, "-f", f)
	}
	if p.project != "" {
		full = append(full, "-p", p.project)
	}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, p.bin, full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker compose %s: %v — %s",
			strings.Join(args, " "), err, conv.Truncate(strings.TrimSpace(string(out)), 300))
	}
	return string(out), nil
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	service, _ := args["service"].(string)
	var out string
	var err error
	switch verb {
	case "up":
		cmdArgs := []string{"up", "-d"}
		// --wait blocks until services are healthy; opt out with wait: false to
		// start a service that is meant to crash or hang (--wait would abort the
		// step before any assertion could observe it).
		if w, ok := args["wait"].(bool); !ok || w {
			cmdArgs = append(cmdArgs, "--wait")
		}
		if list, ok := args["services"].([]any); ok {
			for _, s := range list {
				cmdArgs = append(cmdArgs, fmt.Sprintf("%v", s))
			}
		}
		out, err = p.compose(ctx, cmdArgs...)
	case "down":
		out, err = p.compose(ctx, "down", "-v", "--remove-orphans")
	case "kill":
		out, err = p.compose(ctx, "kill", "-s", "SIGKILL", service)
	case "stop":
		out, err = p.compose(ctx, "stop", service)
	case "start":
		out, err = p.compose(ctx, "start", service)
	case "pause":
		out, err = p.compose(ctx, "pause", service)
	case "unpause":
		out, err = p.compose(ctx, "unpause", service)
	case "logs":
		cmdArgs := []string{"logs", "--no-color"}
		if t, ok := args["tail"]; ok && t != nil && fmt.Sprintf("%v", t) != "" {
			cmdArgs = append(cmdArgs, "--tail", fmt.Sprintf("%v", t))
		}
		if s, ok := args["since"].(string); ok && s != "" {
			cmdArgs = append(cmdArgs, "--since", s)
		}
		cmdArgs = append(cmdArgs, service)
		out, err = p.compose(ctx, cmdArgs...)
	default:
		return sdk.VerbResult{}, fmt.Errorf("docker has no verb %q", verb)
	}
	if err != nil {
		return sdk.VerbResult{Output: out}, err
	}
	return sdk.VerbResult{Value: strings.TrimSpace(out), Output: out}, nil
}
