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
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
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
			// expect: closed inverts the probe: during a partition the port
			// being unreachable is the expected observation, not an error.
			{Name: "expect", Type: "string"},
		},
	}}
}

// classify names how a dial failed — dns (resolution), refused (RST), timeout
// (no answer) — so an assertion can be precise about how the target is down.
func classify(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "refused"
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() || errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "error"
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

	// The explicit timeout arg is authoritative; the 5s default applies only
	// when neither the arg nor the step set a deadline. Nesting the arg inside
	// the default would silently cap it at 5s.
	if t, ok := conv.ToFloat(args["timeout"]); ok && t > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(t*float64(time.Second)))
		defer cancel()
	} else if _, ok := ctx.Deadline(); !ok && p.defaultTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.defaultTimeout)
		defer cancel()
	}

	expect, _ := args["expect"].(string)
	if expect == "" {
		expect = "open"
	}
	if expect != "open" && expect != "closed" {
		return sdk.VerbResult{}, fmt.Errorf("tcp.connect: expect must be \"open\" or \"closed\", got %q", expect)
	}

	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		reason := classify(err)
		meta := map[string]any{"addr": addr, "reason": reason}
		if expect == "closed" {
			// The outage is what the step came to see: closed is a passing
			// observation. Value is "is the port open", so it binds false.
			return sdk.VerbResult{Value: false,
				Output: fmt.Sprintf("%s is closed (%s)", addr, reason), Meta: meta}, nil
		}
		return sdk.VerbResult{Value: false, Meta: meta},
			fmt.Errorf("tcp.connect %s: %s: %w", addr, reason, err)
	}
	_ = conn.Close()
	meta := map[string]any{"addr": addr, "connectMs": elapsed}
	if expect == "closed" {
		return sdk.VerbResult{Value: true, Meta: meta},
			fmt.Errorf("tcp.connect %s: expected closed, but the port accepted in %dms", addr, elapsed)
	}
	return sdk.VerbResult{
		Value:  true,
		Output: fmt.Sprintf("connected to %s in %dms", addr, elapsed),
		Meta:   meta,
	}, nil
}
