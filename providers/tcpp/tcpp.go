// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package tcpp is the tcp built-in provider: a connect/reachability probe.
// It dials a TCP address and reports whether the port is reachable and how
// long the connection took — an L4 health and latency lens for any backend
// (cache, queue, database, gRPC service) regardless of its application
// protocol.
package tcpp

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

// defaultTimeout applies only when a step passes no deadline.
const defaultTimeout = 5 * time.Second

type Provider struct {
	addr           string        // default target, when a step passes no addr
	defaultTimeout time.Duration // applied only when the caller passed no deadline
}

func init() { sdk.Register("tcp", New) }

func New() sdk.Provider { return &Provider{defaultTimeout: defaultTimeout} }

func (p *Provider) Type() string { return "tcp" }

func (p *Provider) Configure(cfg map[string]any) error {
	p.addr = addrOf(cfg)
	if t, ok := conv.ToFloat(cfg["timeout"]); ok && t > 0 {
		p.defaultTimeout = time.Duration(t * float64(time.Second))
	}
	return nil
}

// addrOf resolves a target address from a config/args map: an explicit addr
// ("host:port") wins, otherwise host + port are joined.
func addrOf(m map[string]any) string {
	if a, _ := m["addr"].(string); a != "" {
		return a
	}
	host, _ := m["host"].(string)
	if host == "" {
		return ""
	}
	if port, ok := conv.ToFloat(m["port"]); ok {
		return net.JoinHostPort(host, strconv.Itoa(int(port)))
	}
	if port, _ := m["port"].(string); port != "" {
		return net.JoinHostPort(host, port)
	}
	return host
}

func (p *Provider) Verbs() []sdk.VerbSpec {
	return []sdk.VerbSpec{{
		Name: "connect", Kind: sdk.KindProbe, SideEffects: false, Primary: "addr",
		Args: []sdk.ArgSpec{
			{Name: "addr", Type: "string"},
			{Name: "host", Type: "string"},
			{Name: "port", Type: "number"},
			{Name: "timeout", Type: "number"},
		},
	}}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	if verb != "connect" {
		return sdk.VerbResult{}, fmt.Errorf("tcp has no verb %q", verb)
	}
	addr := addrOf(args)
	if addr == "" {
		addr = p.addr
	}
	if addr == "" {
		return sdk.VerbResult{}, fmt.Errorf("tcp.connect: no addr (set config.addr or pass with: \"host:port\")")
	}

	if _, ok := ctx.Deadline(); !ok && p.defaultTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.defaultTimeout)
		defer cancel()
	}
	if t, ok := conv.ToFloat(args["timeout"]); ok && t > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(t*float64(time.Second)))
		defer cancel()
	}

	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return sdk.VerbResult{}, fmt.Errorf("tcp.connect %s: %w", addr, err)
	}
	_ = conn.Close()
	return sdk.VerbResult{
		Value:  true,
		Output: fmt.Sprintf("connected to %s in %dms", addr, elapsed),
		Meta:   map[string]any{"addr": addr, "connectMs": elapsed},
	}, nil
}
