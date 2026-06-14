// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package toxiproxyp is the toxiproxy built-in provider: proxy-in-path
// network faults, driven through the official Toxiproxy Go client.
package toxiproxyp

import (
	"context"
	"fmt"
	"strings"

	toxiproxy "github.com/Shopify/toxiproxy/v2/client"

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

type Provider struct {
	client *toxiproxy.Client
}

func init() { sdk.Register("toxiproxy", New) }

func New() sdk.Provider { return &Provider{} }

func (p *Provider) Type() string { return "toxiproxy" }

func (p *Provider) Configure(cfg map[string]any) error {
	adminURL := "http://localhost:8474"
	if u, ok := cfg["adminUrl"].(string); ok && u != "" {
		adminURL = strings.TrimRight(u, "/")
	}
	p.client = toxiproxy.NewClient(adminURL)
	return nil
}

func proxyArg() []sdk.ArgSpec {
	return []sdk.ArgSpec{{Name: "proxy", Type: "string", Required: true}}
}

func (p *Provider) Verbs() []sdk.VerbSpec {
	withProxy := func(name string, effect sdk.Effect, extra ...sdk.ArgSpec) sdk.VerbSpec {
		return sdk.VerbSpec{Name: name, Kind: sdk.KindAction, SideEffects: true, Effect: effect,
			Primary: "proxy", Args: append(proxyArg(), extra...)}
	}
	return []sdk.VerbSpec{
		withProxy("add_latency", sdk.EffectDegradation,
			sdk.ArgSpec{Name: "latencyMs", Type: "number", Required: true},
			sdk.ArgSpec{Name: "jitterMs", Type: "number"}),
		withProxy("packet_loss", sdk.EffectOutage, sdk.ArgSpec{Name: "toxicity", Type: "number"}),
		withProxy("bandwidth", sdk.EffectDegradation, sdk.ArgSpec{Name: "rateKbps", Type: "number", Required: true}),
		withProxy("blackhole", sdk.EffectOutage),
		withProxy("partition", sdk.EffectOutage),
		{Name: "reset", Kind: sdk.KindAction, SideEffects: true},
	}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	if verb == "reset" {
		if err := p.client.ResetState(); err != nil {
			return sdk.VerbResult{}, fmt.Errorf("toxiproxy.reset: %w", err)
		}
		return sdk.VerbResult{Value: "reset"}, nil
	}

	name, _ := args["proxy"].(string)
	proxy, err := p.client.Proxy(name)
	if err != nil {
		return sdk.VerbResult{}, fmt.Errorf("toxiproxy.%s: proxy %q: %w", verb, name, err)
	}

	switch verb {
	case "add_latency":
		latency, _ := conv.ToFloat(args["latencyMs"])
		attrs := toxiproxy.Attributes{"latency": latency}
		if j, _ := conv.ToFloat(args["jitterMs"]); j > 0 {
			attrs["jitter"] = j
		}
		_, err = proxy.AddToxic("latency_shinari", "latency", "downstream", 1.0, attrs)
	case "packet_loss":
		toxicity, _ := conv.ToFloat(args["toxicity"])
		if toxicity == 0 {
			toxicity = 1.0
		}
		// timeout-with-0 drops data without closing the connection — the
		// Toxiproxy idiom for packet loss.
		_, err = proxy.AddToxic("packet_loss_shinari", "timeout", "downstream", float32(toxicity), toxiproxy.Attributes{"timeout": 0})
	case "bandwidth":
		rate, _ := conv.ToFloat(args["rateKbps"])
		_, err = proxy.AddToxic("bandwidth_shinari", "bandwidth", "downstream", 1.0, toxiproxy.Attributes{"rate": rate})
	case "blackhole":
		_, err = proxy.AddToxic("blackhole_shinari", "timeout", "downstream", 1.0, toxiproxy.Attributes{"timeout": 0})
	case "partition":
		err = proxy.Disable()
	default:
		return sdk.VerbResult{}, fmt.Errorf("toxiproxy has no verb %q", verb)
	}
	if err != nil {
		return sdk.VerbResult{}, fmt.Errorf("toxiproxy.%s on %q: %w", verb, name, err)
	}
	return sdk.VerbResult{Value: "ok"}, nil
}
