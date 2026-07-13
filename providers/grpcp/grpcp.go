// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package grpcp is the grpc built-in provider: a standard gRPC health-check
// probe. It calls the grpc.health.v1 Health/Check RPC and reports the serving
// status, giving the typed-observation model a probe for gRPC backends that
// would otherwise need an exec escape hatch.
package grpcp

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/shinari-dev/shinari/sdk"
)

// defaultTimeout applies only when the caller passed no deadline — the same
// rule as httpp: a per-step timeout: of any value is authoritative. Without
// it, a health check against a blackholed target would hang for the OS TCP
// timeout.
const defaultTimeout = 30 * time.Second

type Provider struct {
	target         string
	conn           *grpc.ClientConn
	defaultTimeout time.Duration // applied only when the caller passed no deadline
}

func init() { sdk.Register("grpc", New) }

func New() sdk.Provider { return &Provider{defaultTimeout: defaultTimeout} }

func (p *Provider) Type() string { return "grpc" }

// Configure dials the target lazily. grpc.NewClient does not open a connection
// until the first RPC, so (like sql.Open) it is safe to call before setup has
// brought the service up; the real connect happens on the first health check.
func (p *Provider) Configure(cfg map[string]any) error {
	p.target, _ = cfg["target"].(string)
	if p.target == "" {
		p.target, _ = cfg["addr"].(string)
	}
	if p.target == "" {
		return fmt.Errorf("grpc: target is required (host:port)")
	}
	// Insecure by default: the common local/CI case is plaintext. A real
	// transport-credentials path can be added when a scenario needs TLS.
	conn, err := grpc.NewClient(p.target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("grpc: dial %s: %w", p.target, err)
	}
	p.conn = conn
	return nil
}

// Close releases the gRPC channel dialed in Configure.
func (p *Provider) Close() error {
	if p.conn == nil {
		return nil
	}
	return p.conn.Close()
}

func (p *Provider) Verbs() []sdk.VerbSpec {
	return []sdk.VerbSpec{{
		Name: "health", Kind: sdk.KindProbe, SideEffects: false, Primary: "service",
		Args: []sdk.ArgSpec{
			{Name: "service", Type: "string"},
			// expect: down inverts the probe: during the fault window the
			// service reporting NOT_SERVING — or being unreachable at all —
			// is the expected observation, not an error.
			{Name: "expect", Type: "string"},
		},
	}}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	if verb != "health" {
		return sdk.VerbResult{}, fmt.Errorf("grpc has no verb %q", verb)
	}
	if _, ok := ctx.Deadline(); !ok && p.defaultTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.defaultTimeout)
		defer cancel()
	}
	service, _ := args["service"].(string) // "" = the whole server
	expect, _ := args["expect"].(string)
	if expect == "" {
		expect = "serving"
	}
	if expect != "serving" && expect != "down" {
		return sdk.VerbResult{}, fmt.Errorf("grpc.health: expect must be \"serving\" or \"down\", got %q", expect)
	}

	start := time.Now()
	resp, err := grpc_health_v1.NewHealthClient(p.conn).Check(ctx,
		&grpc_health_v1.HealthCheckRequest{Service: service})
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		if expect == "down" {
			// Unreachable is down too: a killed or partitioned backend never
			// answers the RPC, and that is exactly the expected observation.
			status := "UNREACHABLE"
			return sdk.VerbResult{Value: status, Output: err.Error(),
				Meta: map[string]any{"status": status, "rpcMs": elapsed}}, nil
		}
		return sdk.VerbResult{}, fmt.Errorf("grpc.health %s: %w", p.target, err)
	}

	status := resp.GetStatus().String()
	meta := map[string]any{"status": status, "rpcMs": elapsed}
	serving := resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING
	if expect == "down" {
		if serving {
			return sdk.VerbResult{Value: status, Output: status, Meta: meta},
				fmt.Errorf("grpc.health %s: expected down, but it is SERVING", p.target)
		}
		return sdk.VerbResult{Value: status, Output: status, Meta: meta}, nil
	}
	if serving {
		return sdk.VerbResult{Value: status, Output: status, Meta: meta}, nil
	}
	return sdk.VerbResult{Value: status, Output: status, Meta: meta},
		fmt.Errorf("grpc.health %s: not serving: %s", p.target, status)
}
