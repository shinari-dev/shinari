// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package redisp

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

// newTestProvider starts an in-process Redis (miniredis) and configures a
// provider against it — hermetic, no external infra.
func newTestProvider(t *testing.T) sdk.Provider {
	t.Helper()
	mr := miniredis.RunT(t)
	p := New()
	if err := p.Configure(map[string]any{"addr": mr.Addr()}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	return p
}

func TestConfigureRequiresAddrOrURL(t *testing.T) {
	err := New().Configure(map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "addr or url") {
		t.Fatalf("want addr/url error, got %v", err)
	}
}

func TestConfigureAcceptsURL(t *testing.T) {
	if err := New().Configure(map[string]any{"url": "redis://localhost:6379/0"}); err != nil {
		t.Fatalf("url configure: %v", err)
	}
}

func TestSetThenGet(t *testing.T) {
	p := newTestProvider(t)
	ctx := context.Background()
	if _, err := p.Run(ctx, "set", map[string]any{"key": "job:1", "value": "running"}); err != nil {
		t.Fatal(err)
	}
	res, err := p.Run(ctx, "get", map[string]any{"key": "job:1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != "running" {
		t.Fatalf("value = %v, want running", res.Value)
	}
	if res.Meta["hit"] != true {
		t.Errorf("hit meta = %v, want true", res.Meta["hit"])
	}
}

// TestGetMissIsNotError is the cache-miss contract: a missing key returns a nil
// value, not an error, so a scenario can assert a key is gone after an outage.
func TestGetMissIsNotError(t *testing.T) {
	p := newTestProvider(t)
	res, err := p.Run(context.Background(), "get", map[string]any{"key": "absent"})
	if err != nil {
		t.Fatalf("miss should not error: %v", err)
	}
	if res.Value != nil {
		t.Fatalf("value = %v, want nil", res.Value)
	}
	if res.Meta["hit"] != false {
		t.Errorf("hit meta = %v, want false", res.Meta["hit"])
	}
}

func TestDelAndExists(t *testing.T) {
	p := newTestProvider(t)
	ctx := context.Background()
	mustRun(t, p, "set", map[string]any{"key": "a", "value": "1"})
	mustRun(t, p, "set", map[string]any{"key": "b", "value": "2"})

	res, err := p.Run(ctx, "exists", map[string]any{"keys": []any{"a", "b", "c"}})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := conv.ToFloat(res.Value); n != 2 {
		t.Fatalf("exists = %v, want 2", res.Value)
	}

	res, err = p.Run(ctx, "del", map[string]any{"keys": []any{"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := conv.ToFloat(res.Value); n != 2 {
		t.Fatalf("del = %v, want 2", res.Value)
	}
}

func TestCmdGeneric(t *testing.T) {
	p := newTestProvider(t)
	res, err := p.Run(context.Background(), "cmd", map[string]any{"args": []any{"INCR", "n"}})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := conv.ToFloat(res.Value); n != 1 {
		t.Fatalf("INCR = %v, want 1", res.Value)
	}
}

// TestKeysScalarShorthand proves a single scalar is accepted where a list is
// expected (the `with: mykey` shorthand).
func TestKeysScalarShorthand(t *testing.T) {
	p := newTestProvider(t)
	mustRun(t, p, "set", map[string]any{"key": "solo", "value": "x"})
	res, err := p.Run(context.Background(), "del", map[string]any{"keys": "solo"})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := conv.ToFloat(res.Value); n != 1 {
		t.Fatalf("del scalar = %v, want 1", res.Value)
	}
}

func TestPing(t *testing.T) {
	p := newTestProvider(t)
	res, err := p.Run(context.Background(), "ping", nil)
	if err != nil || res.Value != true {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestUnknownVerb(t *testing.T) {
	p := newTestProvider(t)
	if _, err := p.Run(context.Background(), "nope", nil); err == nil {
		t.Fatal("want error for unknown verb")
	}
}

func mustRun(t *testing.T, p sdk.Provider, verb string, args map[string]any) {
	t.Helper()
	if _, err := p.Run(context.Background(), verb, args); err != nil {
		t.Fatalf("%s: %v", verb, err)
	}
}
