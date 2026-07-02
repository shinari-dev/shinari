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
		// clear/reset are the restore side of the three faults above: clear
		// lifts one host's override, reset lifts every shinari-written one.
		// Both are idempotent — restoring a fault that was never injected (a
		// teardown after an early failure) is a no-op, not an error.
		{Name: "clear", Kind: sdk.KindAction, SideEffects: true, Primary: "host", Args: hostArg()},
		{Name: "reset", Kind: sdk.KindAction, SideEffects: true},
	}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	host, _ := args["host"].(string)
	var body string
	switch verb {
	case "clear":
		return p.removeConf(ctx, verb, []string{p.confFile(host)})
	case "reset":
		files, err := filepath.Glob(filepath.Join(p.confDir, "shinari-*.conf"))
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("net.reset: %w", err)
		}
		return p.removeConf(ctx, verb, files)
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

// confFile is the snippet path owned by host: one file per host, so restating
// or clearing a host touches only its own override.
func (p *Provider) confFile(host string) string {
	return filepath.Join(p.confDir, "shinari-"+sanitize(host)+".conf")
}

// reload runs reloadCmd, if configured, so dnsmasq picks up the change.
func (p *Provider) reload(ctx context.Context, verb string) (string, error) {
	if p.reloadCmd == "" {
		return "", nil
	}
	out, err := exec.CommandContext(ctx, "sh", "-c", p.reloadCmd).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("net.%s: reload %q: %w — %s", verb, p.reloadCmd, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// writeConf writes the dnsmasq snippet for host and runs reloadCmd.
func (p *Provider) writeConf(ctx context.Context, verb, host, body string) (sdk.VerbResult, error) {
	file := p.confFile(host)
	if err := os.MkdirAll(p.confDir, 0o755); err != nil {
		return sdk.VerbResult{}, fmt.Errorf("net.%s: %w", verb, err)
	}
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		return sdk.VerbResult{}, fmt.Errorf("net.%s: %w", verb, err)
	}
	out, err := p.reload(ctx, verb)
	if err != nil {
		return sdk.VerbResult{Output: out}, err
	}
	return sdk.VerbResult{Value: file}, nil
}

// removeConf deletes snippet files and reloads: the restore path. A missing
// file is skipped — restoring a fault that was never injected is a no-op —
// and the reload still runs so the resolver converges regardless.
func (p *Provider) removeConf(ctx context.Context, verb string, files []string) (sdk.VerbResult, error) {
	removed := []any{}
	for _, f := range files {
		err := os.Remove(f)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("net.%s: %w", verb, err)
		}
		removed = append(removed, f)
	}
	out, err := p.reload(ctx, verb)
	if err != nil {
		return sdk.VerbResult{Output: out}, err
	}
	return sdk.VerbResult{Value: removed, Output: out, Meta: map[string]any{"removed": len(removed)}}, nil
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' {
			return r
		}
		return '_'
	}, s)
}
