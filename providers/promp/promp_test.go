// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package promp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shinari-dev/shinari/utils/conv"
)

const exposition = `# HELP http_requests_total total requests
# TYPE http_requests_total counter
http_requests_total{method="post",code="200"} 1027
http_requests_total{method="get",code="200"} 5
latency_seconds{quantile="0.99"} 0.15
up 1
`

func scrapeServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(exposition))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestScrapeSelectsByLabels(t *testing.T) {
	s := scrapeServer(t)
	p := New()
	if err := p.Configure(map[string]any{"baseUrl": s.URL}); err != nil {
		t.Fatal(err)
	}
	res, err := p.Run(context.Background(), "scrape", map[string]any{
		"metric": "http_requests_total",
		"labels": map[string]any{"method": "post", "code": "200"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if f, _ := conv.ToFloat(res.Value); f != 1027 {
		t.Fatalf("value = %v", res.Value)
	}
}

func TestScrapeQuantile(t *testing.T) {
	s := scrapeServer(t)
	p := New()
	_ = p.Configure(map[string]any{"baseUrl": s.URL})
	res, err := p.Run(context.Background(), "scrape", map[string]any{
		"metric": "latency_seconds",
		"labels": map[string]any{"quantile": "0.99"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if f, _ := conv.ToFloat(res.Value); f != 0.15 {
		t.Fatalf("value = %v", res.Value)
	}
}

func TestScrapeQuotedLabelEdgeCases(t *testing.T) {
	// legal exposition: label values containing }, comma, and escaped quotes
	labels, ok := parseLabels(`path="/jobs{id},status",name="say \"hi\""`)
	if !ok {
		t.Fatal("parseLabels rejected legal exposition")
	}
	if labels["path"] != `/jobs{id},status` || labels["name"] != `say "hi"` {
		t.Fatalf("labels = %v", labels)
	}
	name, ls, val, ok := parseLine(`requests{path="/a{b},c"} 7`)
	if !ok || name != "requests" || ls["path"] != "/a{b},c" || val != 7 {
		t.Fatalf("parseLine = %q %v %v %v", name, ls, val, ok)
	}
}

func TestScrapeNon200IsError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(s.Close)
	p := New()
	_ = p.Configure(map[string]any{"baseUrl": s.URL})
	_, err := p.Run(context.Background(), "scrape", map[string]any{"metric": "up"})
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("a 500 must surface as an HTTP failure, not a missing metric: %v", err)
	}
}

func TestScrapeMissingIsError(t *testing.T) {
	s := scrapeServer(t)
	p := New()
	_ = p.Configure(map[string]any{"baseUrl": s.URL})
	if _, err := p.Run(context.Background(), "scrape", map[string]any{"metric": "nope"}); err == nil {
		t.Fatal("want error for missing metric")
	}
}

// queryServer serves a canned /api/v1/query response and records the query.
func queryServer(t *testing.T, response string) (*httptest.Server, *string) {
	t.Helper()
	var gotQuery string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(s.Close)
	return s, &gotQuery
}

func queryProvider(t *testing.T, baseURL string) *Provider {
	t.Helper()
	p := New().(*Provider)
	if err := p.Configure(map[string]any{"baseUrl": baseURL}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestQuerySingleSampleBindsFloat(t *testing.T) {
	s, gotQuery := queryServer(t, `{"status":"success","data":{"resultType":"vector",
		"result":[{"metric":{"job":"api"},"value":[1719900000,"0.042"]}]}}`)
	p := queryProvider(t, s.URL)
	res, err := p.Run(context.Background(), "query", map[string]any{
		"query": `rate(http_errors_total{job="api"}[1m])`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != 0.042 {
		t.Errorf("value = %v, want 0.042", res.Value)
	}
	if res.Meta["samples"] != 1 || res.Meta["resultType"] != "vector" {
		t.Errorf("meta = %v", res.Meta)
	}
	if *gotQuery != `rate(http_errors_total{job="api"}[1m])` {
		t.Errorf("server saw query %q", *gotQuery)
	}
}

func TestQueryScalarBindsFloat(t *testing.T) {
	s, _ := queryServer(t, `{"status":"success","data":{"resultType":"scalar","result":[1719900000,"3"]}}`)
	p := queryProvider(t, s.URL)
	res, err := p.Run(context.Background(), "query", map[string]any{"query": "1 + 2"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != float64(3) {
		t.Errorf("value = %v, want 3", res.Value)
	}
}

func TestQueryMultiSampleBindsList(t *testing.T) {
	s, _ := queryServer(t, `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"instance":"a"},"value":[1719900000,"1"]},
		{"metric":{"instance":"b"},"value":[1719900000,"2"]}]}}`)
	p := queryProvider(t, s.URL)
	res, err := p.Run(context.Background(), "query", map[string]any{"query": "up"})
	if err != nil {
		t.Fatal(err)
	}
	list, ok := res.Value.([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("value = %#v, want a two-element list", res.Value)
	}
	first := list[0].(map[string]any)
	if first["value"] != float64(1) {
		t.Errorf("first.value = %v", first["value"])
	}
	if labels := first["labels"].(map[string]any); labels["instance"] != "a" {
		t.Errorf("first.labels = %v", labels)
	}
	if res.Meta["samples"] != 2 {
		t.Errorf("meta.samples = %v, want 2", res.Meta["samples"])
	}
}

func TestQueryEmptyResultIsProbeFailure(t *testing.T) {
	s, _ := queryServer(t, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	p := queryProvider(t, s.URL)
	if _, err := p.Run(context.Background(), "query", map[string]any{"query": "nope"}); err == nil {
		t.Fatal("want error for an empty result")
	}
}

func TestQueryServerErrorSurfaces(t *testing.T) {
	s, _ := queryServer(t, `{"status":"error","errorType":"bad_data","error":"parse error at char 3"}`)
	p := queryProvider(t, s.URL)
	_, err := p.Run(context.Background(), "query", map[string]any{"query": "rate("})
	if err == nil || !strings.Contains(err.Error(), "parse error") {
		t.Fatalf("want the server's error surfaced, got %v", err)
	}
}

func TestQueryMatrixIsUnsupported(t *testing.T) {
	s, _ := queryServer(t, `{"status":"success","data":{"resultType":"matrix","result":[]}}`)
	p := queryProvider(t, s.URL)
	_, err := p.Run(context.Background(), "query", map[string]any{"query": "up[5m]"})
	if err == nil || !strings.Contains(err.Error(), "matrix") {
		t.Fatalf("want a resultType error, got %v", err)
	}
}
