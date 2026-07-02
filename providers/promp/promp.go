// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package promp is the prom built-in provider: scrape a Prometheus/OpenMetrics
// endpoint and select one sample by metric name and labels, or run a PromQL
// instant query against a Prometheus server.
package promp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

// defaultTimeout caps every query/scrape request.
const defaultTimeout = 30 * time.Second

type Provider struct {
	base   string
	client *http.Client
}

func init() { sdk.Register("prom", New) }

func New() sdk.Provider { return &Provider{client: &http.Client{Timeout: defaultTimeout}} }

func (p *Provider) Type() string { return "prom" }

func (p *Provider) Configure(cfg map[string]any) error {
	p.base = conv.BaseURL(cfg)
	return nil
}

func (p *Provider) Verbs() []sdk.VerbSpec {
	return []sdk.VerbSpec{
		{Name: "scrape", Kind: sdk.KindProbe, Primary: "metric", Args: []sdk.ArgSpec{
			{Name: "metric", Type: "string", Required: true},
			{Name: "path", Type: "string"},
			{Name: "labels", Type: "map"},
		}},
		// query evaluates a PromQL expression on a Prometheus server, where
		// scrape reads an exposition endpoint directly. The server does the
		// math (rate, histogram_quantile, aggregations), so an assertion can
		// gate on a derived signal no single exposition line carries.
		{Name: "query", Kind: sdk.KindProbe, Primary: "query", Args: []sdk.ArgSpec{
			{Name: "query", Type: "string", Required: true},
		}},
	}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	if verb == "query" {
		return p.runQuery(ctx, args)
	}
	if verb != "scrape" {
		return sdk.VerbResult{}, fmt.Errorf("prom has no verb %q", verb)
	}
	metric, _ := args["metric"].(string)
	path, _ := args["path"].(string)
	if path == "" {
		path = "/metrics"
	}
	want, _ := args["labels"].(map[string]any)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, conv.JoinURL(p.base, path), nil)
	if err != nil {
		return sdk.VerbResult{}, fmt.Errorf("prom.scrape %s: %w", metric, err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return sdk.VerbResult{}, fmt.Errorf("prom.scrape %s: %w", metric, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if v, ok := selectSample(string(raw), metric, want); ok {
		return sdk.VerbResult{Value: v, Output: string(raw)}, nil
	}
	return sdk.VerbResult{Output: string(raw)},
		fmt.Errorf("prom.scrape: metric %q with the given labels not found", metric)
}

// runQuery evaluates a PromQL instant query via /api/v1/query. A scalar or
// single-sample vector binds its value directly (the shape an aggregated
// expression produces, and what assert wants); a multi-sample vector binds a
// list of { labels, value } maps so read:/capture: can select. An empty
// result is a probe failure, like a scrape miss.
func (p *Provider) runQuery(ctx context.Context, args map[string]any) (sdk.VerbResult, error) {
	expr, _ := args["query"].(string)
	if expr == "" {
		return sdk.VerbResult{}, fmt.Errorf("prom.query needs a query: expression")
	}
	u := conv.JoinURL(p.base, "/api/v1/query") + "?query=" + url.QueryEscape(expr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return sdk.VerbResult{}, fmt.Errorf("prom.query: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return sdk.VerbResult{}, fmt.Errorf("prom.query: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var body struct {
		Status    string `json:"status"`
		ErrorType string `json:"errorType"`
		Error     string `json:"error"`
		Data      struct {
			ResultType string          `json:"resultType"`
			Result     json.RawMessage `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return sdk.VerbResult{Output: string(raw)},
			fmt.Errorf("prom.query: decoding response (status %d): %w", resp.StatusCode, err)
	}
	if body.Status != "success" {
		return sdk.VerbResult{Output: string(raw)},
			fmt.Errorf("prom.query: %s: %s", body.ErrorType, body.Error)
	}

	meta := map[string]any{"resultType": body.Data.ResultType}
	switch body.Data.ResultType {
	case "scalar":
		var point []any
		if err := json.Unmarshal(body.Data.Result, &point); err != nil {
			return sdk.VerbResult{Output: string(raw)}, fmt.Errorf("prom.query: %w", err)
		}
		v, err := pointValue(point)
		if err != nil {
			return sdk.VerbResult{Output: string(raw)}, fmt.Errorf("prom.query: %w", err)
		}
		meta["samples"] = 1
		return sdk.VerbResult{Value: v, Output: string(raw), Meta: meta}, nil
	case "vector":
		var samples []struct {
			Metric map[string]string `json:"metric"`
			Value  []any             `json:"value"`
		}
		if err := json.Unmarshal(body.Data.Result, &samples); err != nil {
			return sdk.VerbResult{Output: string(raw)}, fmt.Errorf("prom.query: %w", err)
		}
		meta["samples"] = len(samples)
		if len(samples) == 0 {
			return sdk.VerbResult{Output: string(raw), Meta: meta},
				fmt.Errorf("prom.query: %q returned no samples", expr)
		}
		if len(samples) == 1 {
			v, err := pointValue(samples[0].Value)
			if err != nil {
				return sdk.VerbResult{Output: string(raw)}, fmt.Errorf("prom.query: %w", err)
			}
			return sdk.VerbResult{Value: v, Output: string(raw), Meta: meta}, nil
		}
		list := make([]any, 0, len(samples))
		for _, s := range samples {
			v, err := pointValue(s.Value)
			if err != nil {
				return sdk.VerbResult{Output: string(raw)}, fmt.Errorf("prom.query: %w", err)
			}
			labels := map[string]any{}
			for k, lv := range s.Metric {
				labels[k] = lv
			}
			list = append(list, map[string]any{"labels": labels, "value": v})
		}
		return sdk.VerbResult{Value: list, Output: string(raw), Meta: meta}, nil
	default:
		// matrix (a range selector without an aggregation) or string: not a
		// point-in-time signal an assertion can gate on.
		return sdk.VerbResult{Output: string(raw), Meta: meta},
			fmt.Errorf("prom.query: resultType %q not supported; use an expression returning an instant vector or scalar", body.Data.ResultType)
	}
}

// pointValue extracts the float from a Prometheus [timestamp, "value"] pair.
func pointValue(pair []any) (float64, error) {
	if len(pair) != 2 {
		return 0, fmt.Errorf("malformed sample %v", pair)
	}
	f, ok := conv.ToFloat(pair[1])
	if !ok {
		return 0, fmt.Errorf("malformed sample value %v", pair[1])
	}
	return f, nil
}

// selectSample finds the first exposition line for metric whose labels include
// all of want, and returns its value.
func selectSample(body, metric string, want map[string]any) (float64, bool) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, labels, val, ok := parseLine(line)
		if !ok || name != metric {
			continue
		}
		if labelsMatch(labels, want) {
			return val, true
		}
	}
	return 0, false
}

func parseLine(line string) (name string, labels map[string]string, val float64, ok bool) {
	var rest string
	if open := strings.IndexByte(line, '{'); open >= 0 {
		closing := strings.IndexByte(line, '}')
		if closing < 0 || closing < open {
			return "", nil, 0, false
		}
		name = strings.TrimSpace(line[:open])
		labels = parseLabels(line[open+1 : closing])
		rest = strings.TrimSpace(line[closing+1:])
	} else {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return "", nil, 0, false
		}
		name = fields[0]
		rest = strings.Join(fields[1:], " ")
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", nil, 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "", nil, 0, false
	}
	return name, labels, v, true
}

func parseLabels(s string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			out[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), `"`)
		}
	}
	return out
}

func labelsMatch(have map[string]string, want map[string]any) bool {
	for k, v := range want {
		if have[k] != conv.ToString(v) {
			return false
		}
	}
	return true
}
