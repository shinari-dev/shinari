// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package loadp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shinari-dev/shinari/sdk"
)

func TestTypeAndVerbSpec(t *testing.T) {
	p := New().(*Provider)
	if p.Type() != "load" {
		t.Fatalf("Type() = %q, want load", p.Type())
	}
	verbs := p.Verbs()
	if len(verbs) != 1 || verbs[0].Name != "run" {
		t.Fatalf("Verbs() = %v, want one verb named run", verbs)
	}
	v := verbs[0]
	if v.Kind != sdk.KindAction {
		t.Errorf("Kind = %q, want action", v.Kind)
	}
	if v.Effect != sdk.EffectNone {
		t.Errorf("Effect = %q, want none (load is workload, not a fault)", v.Effect)
	}
	if !v.SideEffects {
		t.Error("SideEffects = false, want true")
	}
	if v.Primary != "target" {
		t.Errorf("Primary = %q, want target", v.Primary)
	}
}

func TestUnknownVerb(t *testing.T) {
	p := New().(*Provider)
	if _, err := p.Run(context.Background(), "bogus", map[string]any{}); err == nil {
		t.Fatal("expected error for unknown verb")
	}
}

func provider(t *testing.T, baseURL string) *Provider {
	t.Helper()
	p := New().(*Provider)
	if err := p.Configure(map[string]any{"baseUrl": baseURL}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunSuccessWindow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	p := provider(t, srv.URL)

	res, err := p.Run(context.Background(), "run", map[string]any{
		"target": "/health", "rate": 50, "duration": 0.2,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := res.Value.(map[string]any)
	if m["n"].(float64) <= 0 {
		t.Fatalf("n = %v, want > 0", m["n"])
	}
	if m["errorRate"].(float64) != 0 {
		t.Fatalf("errorRate = %v, want 0", m["errorRate"])
	}
	if m["p99"].(float64) < 0 {
		t.Fatalf("p99 = %v, want >= 0", m["p99"])
	}
	if res.Meta["target"] != srv.URL+"/health" {
		t.Fatalf("meta.target = %v", res.Meta["target"])
	}
}

func TestRunCountsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	p := provider(t, srv.URL)

	res, err := p.Run(context.Background(), "run", map[string]any{
		"target": "/x", "rate": 50, "duration": 0.2,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := res.Value.(map[string]any)
	if m["errorRate"].(float64) != 1 {
		t.Fatalf("errorRate = %v, want 1 (all 500s)", m["errorRate"])
	}
}

func TestRunExpectStatusToleratesListedCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()
	p := provider(t, srv.URL)

	// 503 is the declared degraded response: shed load counts as graceful
	// degradation, not as an error.
	res, err := p.Run(context.Background(), "run", map[string]any{
		"target": "/x", "rate": 50, "duration": 0.2, "expectStatus": []any{200, 503},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := res.Value.(map[string]any)
	if m["errorRate"].(float64) != 0 {
		t.Fatalf("errorRate = %v, want 0 (503 tolerated via expectStatus)", m["errorRate"])
	}
}

func TestRunExpectStatusStillCountsUnlistedCodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	p := provider(t, srv.URL)

	res, err := p.Run(context.Background(), "run", map[string]any{
		"target": "/x", "rate": 50, "duration": 0.2, "expectStatus": []any{503},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := res.Value.(map[string]any)
	if m["errorRate"].(float64) != 1 {
		t.Fatalf("errorRate = %v, want 1 (500 is not in expectStatus)", m["errorRate"])
	}
}

func TestRunValidatesArgs(t *testing.T) {
	p := New().(*Provider)
	cases := []map[string]any{
		{"rate": 10, "duration": 1},                        // missing target
		{"target": "http://x", "duration": 1},              // missing rate
		{"target": "http://x", "rate": 10},                 // missing duration
		{"target": "http://x", "rate": 0, "duration": 1},   // non-positive rate
		{"target": "http://x", "rate": 0.5, "duration": 1}, // sub-1 rate would truncate to a vegeta infinite-rate flood
	}
	for i, args := range cases {
		if _, err := p.Run(context.Background(), "run", args); err == nil {
			t.Errorf("case %d: expected validation error for %v", i, args)
		}
	}
}
