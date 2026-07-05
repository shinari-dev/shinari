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

// defaultTimeout applies only when the caller passed no deadline — the same
// rule as httpp: a per-step timeout: of any value is authoritative.
const defaultTimeout = 30 * time.Second

type Provider struct {
	base   string
	client *http.Client
}

func init() { sdk.Register("prom", New) }

func New() sdk.Provider { return &Provider{client: &http.Client{}} }

// boundCtx applies defaultTimeout only when the step set no deadline, so a
// client-level cap can never override an explicit longer step timeout.
func boundCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultTimeout)
}

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

	ctx, cancel := boundCtx(ctx)
	defer cancel()
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
	if resp.StatusCode != http.StatusOK {
		// an error page is not exposition text; reporting "metric not found"
		// over a 500 would mask the real failure
		return sdk.VerbResult{Output: string(raw)},
			fmt.Errorf("prom.scrape %s: endpoint returned status %d", metric, resp.StatusCode)
	}

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
	ctx, cancel := boundCtx(ctx)
	defer cancel()
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
		closing := closeBrace(line, open)
		if closing < 0 {
			return "", nil, 0, false
		}
		name = strings.TrimSpace(line[:open])
		labels, ok = parseLabels(line[open+1 : closing])
		if !ok {
			return "", nil, 0, false
		}
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

// closeBrace finds the `}` closing the label block opened at open, skipping
// quoted label values (which may legally contain `}`, `,`, and escapes).
func closeBrace(s string, open int) int {
	inQuote := false
	for i := open + 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if inQuote {
				i++ // skip the escaped byte
			}
		case '"':
			inQuote = !inQuote
		case '}':
			if !inQuote {
				return i
			}
		}
	}
	return -1
}

// parseLabels scans `k="v",k2="v2"` with exposition escapes (\\ \" \n) so a
// value containing a comma, brace, or quote selects correctly.
func parseLabels(s string) (map[string]string, bool) {
	out := map[string]string{}
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ',' || s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		eq := strings.IndexByte(s[i:], '=')
		if eq < 0 {
			return out, false
		}
		key := strings.TrimSpace(s[i : i+eq])
		i += eq + 1
		if i >= len(s) || s[i] != '"' {
			// tolerate an unquoted value: read to the next comma
			if j := strings.IndexByte(s[i:], ','); j >= 0 {
				out[key] = strings.TrimSpace(s[i : i+j])
				i += j
			} else {
				out[key] = strings.TrimSpace(s[i:])
				i = len(s)
			}
			continue
		}
		i++ // opening quote
		var b strings.Builder
		closed := false
		for i < len(s) {
			c := s[i]
			if c == '\\' && i+1 < len(s) {
				switch s[i+1] {
				case 'n':
					b.WriteByte('\n')
				case '\\':
					b.WriteByte('\\')
				case '"':
					b.WriteByte('"')
				default:
					b.WriteByte(c)
					b.WriteByte(s[i+1])
				}
				i += 2
				continue
			}
			if c == '"' {
				i++
				closed = true
				break
			}
			b.WriteByte(c)
			i++
		}
		if !closed {
			return out, false
		}
		out[key] = b.String()
	}
	return out, true
}

func labelsMatch(have map[string]string, want map[string]any) bool {
	for k, v := range want {
		if have[k] != conv.ToString(v) {
			return false
		}
	}
	return true
}
