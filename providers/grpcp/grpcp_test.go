// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package grpcp

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// healthServer starts a real gRPC server with the standard health service on a
// loopback port and returns its address plus the health server to flip status.
func healthServer(t *testing.T) (string, *health.Server) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	hs := health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, hs)
	go srv.Serve(ln)
	t.Cleanup(srv.Stop)
	return ln.Addr().String(), hs
}

func TestHealthServing(t *testing.T) {
	addr, hs := healthServer(t)
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	p := New()
	if err := p.Configure(map[string]any{"target": addr}); err != nil {
		t.Fatal(err)
	}
	res, err := p.Run(context.Background(), "health", map[string]any{})
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if res.Value != "SERVING" {
		t.Errorf("value = %v, want SERVING", res.Value)
	}
	if _, ok := res.Meta["rpcMs"]; !ok {
		t.Errorf("missing rpcMs meta: %v", res.Meta)
	}
}

func TestHealthNotServingIsError(t *testing.T) {
	addr, hs := healthServer(t)
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	p := New()
	if err := p.Configure(map[string]any{"target": addr}); err != nil {
		t.Fatal(err)
	}
	res, err := p.Run(context.Background(), "health", map[string]any{})
	if err == nil {
		t.Fatalf("expected an error when NOT_SERVING, got %+v", res)
	}
	if res.Value != "NOT_SERVING" {
		t.Errorf("value = %v, want NOT_SERVING (status still surfaced)", res.Value)
	}
}

func TestConfigureRequiresTarget(t *testing.T) {
	if err := New().Configure(map[string]any{}); err == nil {
		t.Fatal("expected an error when target is missing")
	}
}
