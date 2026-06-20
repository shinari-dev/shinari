// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package httpp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs/{job}", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "j-1", "job": r.PathValue("job"), "seconds": r.Form.Get("seconds")})
	})
	mux.HandleFunc("GET /jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{1, 2}})
	})
	mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "kaput", http.StatusInternalServerError)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func provider(t *testing.T, url string) *Provider {
	p := New().(*Provider)
	if err := p.Configure(map[string]any{"baseUrl": url}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPostFormDecodesJSON(t *testing.T) {
	s := server(t)
	p := provider(t, s.URL)
	res, err := p.Run(context.Background(), "post", map[string]any{
		"path": "/jobs/sleep", "form": map[string]any{"seconds": 30},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := res.Value.(map[string]any)
	if m["id"] != "j-1" || m["seconds"] != "30" {
		t.Fatalf("value = %v", m)
	}
}

func TestGet(t *testing.T) {
	s := server(t)
	p := provider(t, s.URL)
	res, err := p.Run(context.Background(), "get", map[string]any{"path": "/jobs?type=sleep"})
	if err != nil {
		t.Fatal(err)
	}
	if items := res.Value.(map[string]any)["items"].([]any); len(items) != 2 {
		t.Fatalf("value = %v", res.Value)
	}
}

func TestStatus400IsFailure(t *testing.T) {
	s := server(t)
	p := provider(t, s.URL)
	_, err := p.Run(context.Background(), "get", map[string]any{"path": "/boom"})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("want status error, got %v", err)
	}
}

func TestApiBaseAlias(t *testing.T) {
	p := New().(*Provider)
	if err := p.Configure(map[string]any{"apiBase": "http://x/"}); err != nil {
		t.Fatal(err)
	}
	if p.baseURL != "http://x" {
		t.Fatalf("baseURL = %q", p.baseURL)
	}
}

func TestGetSetsStatusAndBytesMeta(t *testing.T) {
	s := server(t)
	p := provider(t, s.URL)
	res, err := p.Run(context.Background(), "get", map[string]any{"path": "/jobs"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta["status"].(int) != 200 {
		t.Fatalf("meta.status = %v", res.Meta["status"])
	}
	if b, _ := res.Meta["bytes"].(int); b <= 0 {
		t.Fatalf("meta.bytes = %v", res.Meta["bytes"])
	}
}

func TestExpectStatusAcceptsListedCode(t *testing.T) {
	s := server(t)
	p := provider(t, s.URL)
	res, err := p.Run(context.Background(), "get", map[string]any{
		"path": "/boom", "expectStatus": []any{200, 500},
	})
	if err != nil {
		t.Fatalf("500 should be accepted via expectStatus, got err %v", err)
	}
	if res.Meta["status"].(int) != 500 {
		t.Fatalf("meta.status = %v", res.Meta["status"])
	}
}

func TestContextDeadlineGovernsTimeout(t *testing.T) {
	// A server that delays longer than the step's deadline.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	p := New()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := p.(*Provider).Run(ctx, "get", map[string]any{"path": srv.URL})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error should unwrap to context.DeadlineExceeded, got %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("request should have aborted at ~100ms, took %s (client 30s cap still in force?)", elapsed)
	}
}

func TestNoDeadlineFallsBackToDefault(t *testing.T) {
	// With no caller deadline the provider must apply its own default so a hung
	// server cannot block forever. (HTTP does not carry a client deadline to the
	// server, so we observe the fallback by shrinking defaultTimeout and pointing
	// at a slow server.)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	p := New().(*Provider)
	p.defaultTimeout = 50 * time.Millisecond

	start := time.Now()
	_, err := p.Run(context.Background(), "get", map[string]any{"path": srv.URL})
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("with no caller deadline the provider's default must apply, got %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("default deadline (~50ms) did not apply, took %s", elapsed)
	}
}

func TestRawBodyWithContentType(t *testing.T) {
	var gotCT, gotBody string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.Close)
	p := provider(t, s.URL)
	if _, err := p.Run(context.Background(), "post", map[string]any{
		"path": "/flows", "raw": "id: hello\nnamespace: demo\n", "contentType": "application/x-yaml",
	}); err != nil {
		t.Fatal(err)
	}
	if gotCT != "application/x-yaml" {
		t.Errorf("content-type = %q, want application/x-yaml", gotCT)
	}
	if gotBody != "id: hello\nnamespace: demo\n" {
		t.Errorf("body = %q (raw must not be JSON-marshalled)", gotBody)
	}
}

func TestBasicAuthFromConfig(t *testing.T) {
	var user, pass string
	var ok bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.Close)
	p := New().(*Provider)
	if err := p.Configure(map[string]any{
		"baseUrl":   s.URL,
		"basicAuth": map[string]any{"username": "admin", "password": "s3cret"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Run(context.Background(), "get", map[string]any{"path": "/"}); err != nil {
		t.Fatal(err)
	}
	if !ok || user != "admin" || pass != "s3cret" {
		t.Errorf("basic auth = %q/%q (ok=%v), want admin/s3cret", user, pass, ok)
	}
}

func TestPerStepBasicAuthOverridesConfig(t *testing.T) {
	var user string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, _ = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.Close)
	p := New().(*Provider)
	if err := p.Configure(map[string]any{
		"baseUrl":   s.URL,
		"basicAuth": map[string]any{"username": "admin", "password": "x"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Run(context.Background(), "get", map[string]any{
		"path": "/", "basicAuth": map[string]any{"username": "tenant", "password": "y"},
	}); err != nil {
		t.Fatal(err)
	}
	if user != "tenant" {
		t.Errorf("basic auth user = %q, want per-step override 'tenant'", user)
	}
}
