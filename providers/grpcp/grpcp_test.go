// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package grpcp

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

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

func TestExpectDownPassesOnNotServing(t *testing.T) {
	addr, hs := healthServer(t)
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	p := New()
	if err := p.Configure(map[string]any{"target": addr}); err != nil {
		t.Fatal(err)
	}
	res, err := p.Run(context.Background(), "health", map[string]any{"expect": "down"})
	if err != nil {
		t.Fatalf("NOT_SERVING must be a passing observation under expect: down, got %v", err)
	}
	if res.Value != "NOT_SERVING" {
		t.Errorf("value = %v, want NOT_SERVING", res.Value)
	}
}

func TestExpectDownPassesOnUnreachable(t *testing.T) {
	// Bind then close: the RPC cannot even connect — a killed or partitioned
	// backend looks exactly like this, and it is the expected observation.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	p := New().(*Provider)
	p.defaultTimeout = 2 * time.Second
	if err := p.Configure(map[string]any{"target": addr}); err != nil {
		t.Fatal(err)
	}
	res, err := p.Run(context.Background(), "health", map[string]any{"expect": "down"})
	if err != nil {
		t.Fatalf("unreachable must be a passing observation under expect: down, got %v", err)
	}
	if res.Value != "UNREACHABLE" || res.Meta["status"] != "UNREACHABLE" {
		t.Errorf("value = %v meta = %v, want UNREACHABLE", res.Value, res.Meta)
	}
}

func TestExpectDownFailsOnServing(t *testing.T) {
	addr, hs := healthServer(t)
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	p := New()
	if err := p.Configure(map[string]any{"target": addr}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Run(context.Background(), "health", map[string]any{"expect": "down"}); err == nil {
		t.Fatal("a SERVING backend must fail a step that expects down")
	}
}

func TestNoDeadlineFallsBackToDefault(t *testing.T) {
	// A listener that accepts (kernel-level) but never speaks HTTP/2: the
	// health RPC hangs at the transport, so without its own default deadline
	// the provider would block for the OS TCP timeout.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	p := New().(*Provider)
	p.defaultTimeout = 100 * time.Millisecond
	if err := p.Configure(map[string]any{"target": ln.Addr().String()}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = p.Run(context.Background(), "health", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "DeadlineExceeded") {
		t.Fatalf("want the default deadline surfaced, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("default deadline (~100ms) did not apply, took %s", elapsed)
	}
}

func TestConfigureRequiresTarget(t *testing.T) {
	if err := New().Configure(map[string]any{}); err == nil {
		t.Fatal("expected an error when target is missing")
	}
}
