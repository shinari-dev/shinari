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
	"github.com/shinari-dev/shinari/utils/conv"
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
			Args: append(hostArg(),
				sdk.ArgSpec{Name: "ip", Type: "string"},
				sdk.ArgSpec{Name: "ips", Type: "list"})},
		{Name: "nxdomain", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "host", Args: hostArg()},
		{Name: "dns_blackhole", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "host", Args: hostArg()},
	}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	host, _ := args["host"].(string)
	var body string
	switch verb {
	case "set_dns":
		// The name resolves to exactly this set of addresses: one
		// address= line per address, served as N A records.
		addrs := addresses(args)
		if len(addrs) == 0 {
			return sdk.VerbResult{}, fmt.Errorf("net.set_dns: needs ip or ips; use net.nxdomain for no records")
		}
		var b strings.Builder
		for _, ip := range addrs {
			fmt.Fprintf(&b, "address=/%s/%s\n", host, ip)
		}
		body = b.String()
	case "nxdomain":
		// empty address= returns NXDOMAIN for the domain
		body = fmt.Sprintf("address=/%s/\n", host)
	case "dns_blackhole":
		// 0.0.0.0 resolves but routes nowhere
		body = fmt.Sprintf("address=/%s/0.0.0.0\n", host)
	default:
		return sdk.VerbResult{}, fmt.Errorf("net has no verb %q", verb)
	}
	return p.writeConf(ctx, verb, host, body)
}

// addresses collects the set the name should resolve to, unioning the
// scalar ip with the ips list, preserving order and de-duplicating.
func addresses(args map[string]any) []string {
	var out []string
	seen := map[string]bool{}
	add := func(ip string) {
		if ip == "" || seen[ip] {
			return
		}
		seen[ip] = true
		out = append(out, ip)
	}
	if list, ok := args["ips"].([]any); ok {
		for _, v := range list {
			add(conv.ToString(v))
		}
	}
	if ip, ok := args["ip"].(string); ok {
		add(ip)
	}
	return out
}

// writeConf writes the dnsmasq snippet for host and runs reloadCmd.
func (p *Provider) writeConf(ctx context.Context, verb, host, body string) (sdk.VerbResult, error) {
	file := filepath.Join(p.confDir, "shinari-"+sanitize(host)+".conf")
	if err := os.MkdirAll(p.confDir, 0o755); err != nil {
		return sdk.VerbResult{}, fmt.Errorf("net.%s: %w", verb, err)
	}
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
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
