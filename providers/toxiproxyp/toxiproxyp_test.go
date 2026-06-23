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
	mu      sync.Mutex
	toxics  []map[string]any
	posts   []string
	removed []string
	resets  int
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
	mux.HandleFunc("GET /proxies/app/toxics", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.toxics)
	})
	mux.HandleFunc("DELETE /proxies/app/toxics/{name}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.removed = append(f.removed, r.PathValue("name"))
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
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
	// with no direction, a toxic defaults to the server→client stream,
	// preserving the original single-direction behavior.
	if f.toxics[0]["stream"] != "downstream" {
		t.Errorf("default stream = %v, want downstream", f.toxics[0]["stream"])
	}
}

func TestDirectionToServerFaultsUpstream(t *testing.T) {
	f := &fakeAdmin{}
	p := provider(t, f.server(t).URL)
	// to_server is the client→service leg — the path a worker uses to send
	// results back, which downstream-only faults cannot reach.
	if _, err := p.Run(context.Background(), "add_latency",
		map[string]any{"proxy": "app", "latencyMs": 100, "direction": "to_server"}); err != nil {
		t.Fatal(err)
	}
	if len(f.toxics) != 1 || f.toxics[0]["stream"] != "upstream" {
		t.Fatalf("toxics = %v, want a single upstream toxic", f.toxics)
	}
}

func TestDirectionUpstreamAlias(t *testing.T) {
	f := &fakeAdmin{}
	p := provider(t, f.server(t).URL)
	// upstream/downstream are accepted as toxiproxy-native aliases.
	if _, err := p.Run(context.Background(), "blackhole",
		map[string]any{"proxy": "app", "direction": "upstream"}); err != nil {
		t.Fatal(err)
	}
	if f.toxics[0]["stream"] != "upstream" {
		t.Fatalf("stream = %v, want upstream", f.toxics[0]["stream"])
	}
}

func TestDirectionBothInstallsOnEachStream(t *testing.T) {
	f := &fakeAdmin{}
	p := provider(t, f.server(t).URL)
	if _, err := p.Run(context.Background(), "add_latency",
		map[string]any{"proxy": "app", "latencyMs": 100, "direction": "both"}); err != nil {
		t.Fatal(err)
	}
	if len(f.toxics) != 2 {
		t.Fatalf("want a toxic on each stream, got %d: %v", len(f.toxics), f.toxics)
	}
	streams := map[any]bool{f.toxics[0]["stream"]: true, f.toxics[1]["stream"]: true}
	if !streams["upstream"] || !streams["downstream"] {
		t.Errorf("streams = %v, want both upstream and downstream", streams)
	}
	// distinct names so the two toxics do not collide on the same proxy.
	if f.toxics[0]["name"] == f.toxics[1]["name"] {
		t.Errorf("toxic names collide: %v", f.toxics[0]["name"])
	}
}

func TestUnknownDirectionErrors(t *testing.T) {
	f := &fakeAdmin{}
	p := provider(t, f.server(t).URL)
	_, err := p.Run(context.Background(), "add_latency",
		map[string]any{"proxy": "app", "latencyMs": 1, "direction": "sideways"})
	if err == nil {
		t.Fatal("want error for unknown direction")
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

func TestTimeoutSetsConfigurableTimeout(t *testing.T) {
	f := &fakeAdmin{}
	p := provider(t, f.server(t).URL)
	if _, err := p.Run(context.Background(), "timeout", map[string]any{"proxy": "app", "timeoutMs": 2000}); err != nil {
		t.Fatal(err)
	}
	if f.toxics[0]["type"] != "timeout" {
		t.Fatalf("toxics = %v", f.toxics)
	}
	attrs := f.toxics[0]["attributes"].(map[string]any)
	if attrs["timeout"] != float64(2000) {
		t.Errorf("timeout attr = %v, want a non-zero cut-then-close interval", attrs["timeout"])
	}
}

func TestClearRemovesToxicsAndReenablesProxy(t *testing.T) {
	f := &fakeAdmin{}
	p := provider(t, f.server(t).URL)
	if _, err := p.Run(context.Background(), "blackhole", map[string]any{"proxy": "app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Run(context.Background(), "clear", map[string]any{"proxy": "app"}); err != nil {
		t.Fatal(err)
	}
	// the proxy's own toxic is removed, scoped to this proxy...
	if len(f.removed) != 1 || f.removed[0] != "blackhole_shinari_downstream" {
		t.Fatalf("removed = %v, want only this proxy's toxic", f.removed)
	}
	// ...and the proxy is re-enabled (undoing a partition) via an update POST.
	if len(f.posts) == 0 {
		t.Fatalf("expected a re-enable update, posts = %v", f.posts)
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
