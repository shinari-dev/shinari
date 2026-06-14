// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package netp is the net built-in provider: DNS-level faults via dnsmasq
// conf snippets. It writes one snippet per host into confDir and runs
// reloadCmd — the environment decides how dnsmasq is actually reloaded
// (SIGHUP, container restart, ...).
package netp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shinari-dev/shinari/sdk"
)

type Provider struct {
	confDir   string
	reloadCmd string
}

func init() { sdk.Register("net", New) }

func New() sdk.Provider { return &Provider{} }

func (p *Provider) Type() string { return "net" }

func (p *Provider) Configure(cfg map[string]any) error {
	if d, ok := cfg["confDir"].(string); ok {
		p.confDir = d
	}
	if c, ok := cfg["reloadCmd"].(string); ok {
		p.reloadCmd = c
	}
	if p.confDir == "" {
		return fmt.Errorf("net provider needs config confDir (where dnsmasq conf snippets are written)")
	}
	return nil
}

func hostArg() []sdk.ArgSpec {
	return []sdk.ArgSpec{{Name: "host", Type: "string", Required: true}}
}

func (p *Provider) Verbs() []sdk.VerbSpec {
	return []sdk.VerbSpec{
		{Name: "set_dns", Kind: sdk.KindAction, SideEffects: true, Primary: "host",
			Args: append(hostArg(), sdk.ArgSpec{Name: "ip", Type: "string", Required: true})},
		{Name: "nxdomain", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "host", Args: hostArg()},
		{Name: "dns_blackhole", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "host", Args: hostArg()},
	}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	host, _ := args["host"].(string)
	var line string
	switch verb {
	case "set_dns":
		ip, _ := args["ip"].(string)
		line = fmt.Sprintf("address=/%s/%s\n", host, ip)
	case "nxdomain":
		// empty address= returns NXDOMAIN for the domain
		line = fmt.Sprintf("address=/%s/\n", host)
	case "dns_blackhole":
		// 0.0.0.0 resolves but routes nowhere
		line = fmt.Sprintf("address=/%s/0.0.0.0\n", host)
	default:
		return sdk.VerbResult{}, fmt.Errorf("net has no verb %q", verb)
	}

	file := filepath.Join(p.confDir, "shinari-"+sanitize(host)+".conf")
	if err := os.MkdirAll(p.confDir, 0o755); err != nil {
		return sdk.VerbResult{}, fmt.Errorf("net.%s: %w", verb, err)
	}
	if err := os.WriteFile(file, []byte(line), 0o644); err != nil {
		return sdk.VerbResult{}, fmt.Errorf("net.%s: %w", verb, err)
	}
	if p.reloadCmd != "" {
		out, err := exec.CommandContext(ctx, "sh", "-c", p.reloadCmd).CombinedOutput()
		if err != nil {
			return sdk.VerbResult{Output: string(out)},
				fmt.Errorf("net.%s: reload %q: %v — %s", verb, p.reloadCmd, err, strings.TrimSpace(string(out)))
		}
	}
	return sdk.VerbResult{Value: file}, nil
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' {
			return r
		}
		return '_'
	}, s)
}
