// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package promp is the prom built-in provider: scrape a Prometheus/OpenMetrics
// endpoint and select one sample by metric name and labels.
package promp

import (
	"context"
	"fmt"
	"io"
	"net/http"
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
	return []sdk.VerbSpec{{Name: "scrape", Kind: sdk.KindProbe, Primary: "metric", Args: []sdk.ArgSpec{
		{Name: "metric", Type: "string", Required: true},
		{Name: "path", Type: "string"},
		{Name: "labels", Type: "map"},
	}}}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
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
	rest := line
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
