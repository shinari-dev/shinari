// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package loadp is the load built-in provider: it generates controlled HTTP
// workload and returns the same { n, errors, errorRate, min, max, mean, p50,
// p95, p99 } window statistics as the `sample` builtin. It is the input side of
// a degradation assertion — drive traffic while a fault is active, then assert
// the system stayed fast enough. The HTTP engine is an implementation detail and
// is never exposed in the verb surface.
package loadp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
	"github.com/shinari-dev/shinari/utils/stats"
	vegeta "github.com/tsenart/vegeta/v12/lib"
)

// Provider generates HTTP workload against a target.
type Provider struct {
	baseURL string
}

func init() { sdk.Register("load", New) }

// New constructs an unconfigured load provider.
func New() sdk.Provider { return &Provider{} }

func (p *Provider) Type() string { return "load" }

// Configure accepts an optional baseUrl prepended to relative targets.
func (p *Provider) Configure(cfg map[string]any) error {
	for _, key := range []string{"baseUrl", "apiBase"} {
		if v, ok := cfg[key].(string); ok && v != "" {
			p.baseURL = strings.TrimRight(v, "/")
			return nil
		}
	}
	return nil
}

// Verbs declares load.run: a blocking action (Effect none — load is workload,
// not a fault). Concurrency with fault injection is provided by the `parallel`
// block, so this verb owns no start/stop lifecycle of its own.
func (p *Provider) Verbs() []sdk.VerbSpec {
	return []sdk.VerbSpec{{
		Name:        "run",
		Kind:        sdk.KindAction,
		SideEffects: true,
		Effect:      sdk.EffectNone,
		Primary:     "target",
		Args: []sdk.ArgSpec{
			{Name: "target", Type: "string", Required: true},
			{Name: "rate", Type: "number", Required: true},
			{Name: "duration", Type: "number", Required: true},
			{Name: "method", Type: "string"},
			{Name: "headers", Type: "map"},
			{Name: "body", Type: "any"},
		},
	}}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	if verb != "run" {
		return sdk.VerbResult{}, fmt.Errorf("load: unknown verb %q", verb)
	}

	target := conv.ToString(args["target"])
	if target == "" {
		return sdk.VerbResult{}, fmt.Errorf("load.run needs target:")
	}
	rate, ok := conv.ToFloat(args["rate"])
	if !ok || rate < 1 {
		// vegeta's ConstantPacer treats Freq == 0 as "infinite rate", so a
		// fractional rate truncated to 0 would flood the target instead of
		// throttling it. Require at least one request per second.
		return sdk.VerbResult{}, fmt.Errorf("load.run needs rate: (requests per second, >= 1)")
	}
	dur, ok := conv.ToFloat(args["duration"])
	if !ok || dur <= 0 {
		return sdk.VerbResult{}, fmt.Errorf("load.run needs duration: (seconds, > 0)")
	}

	method := http.MethodGet
	if m := conv.ToString(args["method"]); m != "" {
		method = strings.ToUpper(m)
	}
	header := http.Header{}
	if hm, ok := args["headers"].(map[string]any); ok {
		for k, v := range hm {
			header.Set(k, conv.ToString(v))
		}
	}
	var body []byte
	if b, ok := args["body"]; ok && b != nil {
		if s, isStr := b.(string); isStr {
			body = []byte(s)
		} else {
			encoded, err := json.Marshal(b)
			if err != nil {
				return sdk.VerbResult{}, fmt.Errorf("load.run: encode body: %w", err)
			}
			body = encoded
			if header.Get("Content-Type") == "" {
				header.Set("Content-Type", "application/json")
			}
		}
	}

	url := p.resolve(target)
	targeter := vegeta.NewStaticTargeter(vegeta.Target{
		Method: method, URL: url, Header: header, Body: body,
	})
	attacker := vegeta.NewAttacker()
	pacer := vegeta.Rate{Freq: int(rate + 0.5), Per: time.Second} // round; rate >= 1 guarantees Freq >= 1
	duration := time.Duration(dur * float64(time.Second))

	// Honor cancellation: stop the attack if the engine cancels the step.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			attacker.Stop()
		case <-stop:
		}
	}()

	var lats []float64
	errs := 0
	for res := range attacker.Attack(targeter, pacer, duration, "shinari-load") {
		lats = append(lats, float64(res.Latency)/float64(time.Millisecond))
		if res.Error != "" || res.Code >= 400 {
			errs++
		}
	}
	attacker.Stop()

	meta := map[string]any{
		"target":      url,
		"rate":        rate,
		"durationSec": dur,
	}
	return sdk.VerbResult{Value: stats.Summarize(lats, errs), Meta: meta}, nil
}

// resolve joins a relative target onto the configured baseURL; absolute URLs
// pass through unchanged.
func (p *Provider) resolve(target string) string {
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return target
	}
	if p.baseURL == "" {
		return target
	}
	return p.baseURL + "/" + strings.TrimLeft(target, "/")
}
