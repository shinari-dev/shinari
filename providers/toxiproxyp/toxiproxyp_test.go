// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package toxiproxyp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// fakeAdmin emulates the Toxiproxy admin API surface the client touches.
type fakeAdmin struct {
	mu     sync.Mutex
	toxics []map[string]any
	posts  []string
	resets int
}

func (f *fakeAdmin) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	proxyJSON := map[string]any{
		"name": "app", "listen": "127.0.0.1:21212", "upstream": "app:8080",
		"enabled": true, "toxics": []any{},
	}
	mux.HandleFunc("GET /proxies/app", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(proxyJSON)
	})
	mux.HandleFunc("POST /proxies/app", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.posts = append(f.posts, "update")
		f.mu.Unlock()
		proxyJSON["enabled"] = body["enabled"]
		_ = json.NewEncoder(w).Encode(proxyJSON)
	})
	mux.HandleFunc("POST /proxies/app/toxics", func(w http.ResponseWriter, r *http.Request) {
		var toxic map[string]any
		_ = json.NewDecoder(r.Body).Decode(&toxic)
		f.mu.Lock()
		f.toxics = append(f.toxics, toxic)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(toxic)
	})
	mux.HandleFunc("POST /reset", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.resets++
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /proxies/ghost", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"proxy not found","status":404}`, http.StatusNotFound)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func provider(t *testing.T, url string) *Provider {
	p := New().(*Provider)
	if err := p.Configure(map[string]any{"adminUrl": url}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAddLatency(t *testing.T) {
	f := &fakeAdmin{}
	p := provider(t, f.server(t).URL)
	_, err := p.Run(context.Background(), "add_latency",
		map[string]any{"proxy": "app", "latencyMs": 500, "jitterMs": 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.toxics) != 1 || f.toxics[0]["type"] != "latency" {
		t.Fatalf("toxics = %v", f.toxics)
	}
	attrs := f.toxics[0]["attributes"].(map[string]any)
	if attrs["latency"] != float64(500) || attrs["jitter"] != float64(50) {
		t.Errorf("attrs = %v", attrs)
	}
}

func TestBlackholeIsTimeoutZero(t *testing.T) {
	f := &fakeAdmin{}
	p := provider(t, f.server(t).URL)
	if _, err := p.Run(context.Background(), "blackhole", map[string]any{"proxy": "app"}); err != nil {
		t.Fatal(err)
	}
	if f.toxics[0]["type"] != "timeout" {
		t.Fatalf("toxics = %v", f.toxics)
	}
}

func TestPartitionDisablesProxy(t *testing.T) {
	f := &fakeAdmin{}
	p := provider(t, f.server(t).URL)
	if _, err := p.Run(context.Background(), "partition", map[string]any{"proxy": "app"}); err != nil {
		t.Fatal(err)
	}
	if len(f.posts) != 1 {
		t.Fatalf("posts = %v", f.posts)
	}
}

func TestReset(t *testing.T) {
	f := &fakeAdmin{}
	p := provider(t, f.server(t).URL)
	if _, err := p.Run(context.Background(), "reset", nil); err != nil {
		t.Fatal(err)
	}
	if f.resets != 1 {
		t.Fatalf("resets = %d", f.resets)
	}
}

func TestUnknownProxyNamesIt(t *testing.T) {
	f := &fakeAdmin{}
	p := provider(t, f.server(t).URL)
	_, err := p.Run(context.Background(), "add_latency", map[string]any{"proxy": "ghost", "latencyMs": 1})
	if err == nil {
		t.Fatal("want error for unknown proxy")
	}
}
