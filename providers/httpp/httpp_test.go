// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package httpp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
