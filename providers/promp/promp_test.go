// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package promp

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestScrapeMissingIsError(t *testing.T) {
	s := scrapeServer(t)
	p := New()
	_ = p.Configure(map[string]any{"baseUrl": s.URL})
	if _, err := p.Run(context.Background(), "scrape", map[string]any{"metric": "nope"}); err == nil {
		t.Fatal("want error for missing metric")
	}
}
