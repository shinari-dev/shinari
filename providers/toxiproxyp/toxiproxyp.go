// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package toxiproxyp is the toxiproxy built-in provider: proxy-in-path
// network faults, driven through the official Toxiproxy Go client.
package toxiproxyp

import (
	"context"
	"errors"
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

// resolveStreams maps a direction selector onto the toxiproxy stream(s) a toxic
// is installed on. Direction is named by role relative to the proxied service:
// from_server (downstream, the default) faults the service→client path;
// to_server (upstream) faults the client→service path — the leg a worker uses
// to send results in, unreachable by downstream-only faults. "both" installs
// the toxic on each stream. The toxiproxy-native upstream/downstream names are
// accepted as aliases.
func resolveStreams(arg any) ([]string, error) {
	dir, _ := arg.(string)
	switch dir {
	case "", "from_server", "downstream":
		return []string{"downstream"}, nil
	case "to_server", "upstream":
		return []string{"upstream"}, nil
	case "both":
		return []string{"upstream", "downstream"}, nil
	default:
		return nil, fmt.Errorf("unknown direction %q (want to_server | from_server | both)", dir)
	}
}

func (p *Provider) Verbs() []sdk.VerbSpec {
	withProxy := func(name string, effect sdk.Effect, extra ...sdk.ArgSpec) sdk.VerbSpec {
		return sdk.VerbSpec{Name: name, Kind: sdk.KindAction, SideEffects: true, Effect: effect,
			Primary: "proxy", Args: append(proxyArg(), extra...)}
	}
	// toxic-based faults take a direction selector; connection-level verbs
	// (partition/clear/reset) act on both streams at once and ignore it.
	withToxic := func(name string, effect sdk.Effect, extra ...sdk.ArgSpec) sdk.VerbSpec {
		return withProxy(name, effect, append(extra, sdk.ArgSpec{Name: "direction", Type: "string"})...)
	}
	return []sdk.VerbSpec{
		withToxic("add_latency", sdk.EffectDegradation,
			sdk.ArgSpec{Name: "latencyMs", Type: "number", Required: true},
			sdk.ArgSpec{Name: "jitterMs", Type: "number"}),
		withToxic("packet_loss", sdk.EffectOutage, sdk.ArgSpec{Name: "toxicity", Type: "number"}),
		withToxic("bandwidth", sdk.EffectDegradation, sdk.ArgSpec{Name: "rateKbps", Type: "number", Required: true}),
		withToxic("blackhole", sdk.EffectOutage),
		withToxic("timeout", sdk.EffectOutage, sdk.ArgSpec{Name: "timeoutMs", Type: "number", Required: true}),
		withProxy("partition", sdk.EffectOutage),
		withProxy("clear", sdk.EffectNone),
		{Name: "reset", Kind: sdk.KindAction, SideEffects: true},
	}
}

// call runs fn under the step's context. The Shopify client offers no context
// support (its HTTP client has a fixed 30s timeout), so a step deadline or
// cancellation stops the wait here; the abandoned call is bounded by that
// client-side timeout. Without this, a hung admin API would stall the step
// past any engine deadline — on the provider whose job is network faults.
func call(ctx context.Context, fn func() error) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	if verb == "reset" {
		if err := call(ctx, p.client.ResetState); err != nil {
			return sdk.VerbResult{}, fmt.Errorf("toxiproxy.reset: %w", err)
		}
		return sdk.VerbResult{Value: "reset"}, nil
	}

	name, _ := args["proxy"].(string)
	var proxy *toxiproxy.Proxy
	if err := call(ctx, func() error {
		var perr error
		proxy, perr = p.client.Proxy(name)
		return perr
	}); err != nil {
		return sdk.VerbResult{}, fmt.Errorf("toxiproxy.%s: proxy %q: %w", verb, name, err)
	}

	// addToxic installs one toxic on each resolved stream, suffixing its name
	// with the stream so the two directions never collide on the same proxy.
	streams, err := resolveStreams(args["direction"])
	if err != nil {
		return sdk.VerbResult{}, fmt.Errorf("toxiproxy.%s on %q: %w", verb, name, err)
	}
	addToxic := func(base, typ string, toxicity float32, attrs toxiproxy.Attributes) error {
		for _, stream := range streams {
			if _, e := proxy.AddToxic(base+"_shinari_"+stream, typ, stream, toxicity, attrs); e != nil {
				return e
			}
		}
		return nil
	}

	var work func() error
	switch verb {
	case "add_latency":
		latency, _ := conv.ToFloat(args["latencyMs"])
		attrs := toxiproxy.Attributes{"latency": latency}
		if j, _ := conv.ToFloat(args["jitterMs"]); j > 0 {
			attrs["jitter"] = j
		}
		work = func() error { return addToxic("latency", "latency", 1.0, attrs) }
	case "packet_loss":
		toxicity, _ := conv.ToFloat(args["toxicity"])
		if toxicity == 0 {
			toxicity = 1.0
		}
		// timeout-with-0 drops data without closing the connection — the
		// Toxiproxy idiom for packet loss.
		work = func() error {
			return addToxic("packet_loss", "timeout", float32(toxicity), toxiproxy.Attributes{"timeout": 0})
		}
	case "bandwidth":
		rate, _ := conv.ToFloat(args["rateKbps"])
		work = func() error { return addToxic("bandwidth", "bandwidth", 1.0, toxiproxy.Attributes{"rate": rate}) }
	case "blackhole":
		work = func() error { return addToxic("blackhole", "timeout", 1.0, toxiproxy.Attributes{"timeout": 0}) }
	case "timeout":
		ms, _ := conv.ToFloat(args["timeoutMs"])
		// a non-zero timeout drops all data, then closes the connection after
		// the interval — a link that wedges and is torn down after a bounded
		// wait, distinct from blackhole (timeout 0), which never closes.
		work = func() error { return addToxic("timeout", "timeout", 1.0, toxiproxy.Attributes{"timeout": ms}) }
	case "partition":
		work = proxy.Disable
	case "clear":
		// scoped restore: remove this proxy's toxics and re-enable it (undoing a
		// partition), leaving every other proxy untouched — unlike reset, which
		// clears toxics and re-enables proxies globally. Re-enables even when a
		// toxic removal fails (aggregating errors), so a partial failure never
		// leaves the system partitioned.
		work = func() error {
			var errs []error
			if toxics, terr := proxy.Toxics(); terr == nil {
				for _, tx := range toxics {
					if rerr := proxy.RemoveToxic(tx.Name); rerr != nil {
						errs = append(errs, rerr)
					}
				}
			} else {
				errs = append(errs, terr)
			}
			if eerr := proxy.Enable(); eerr != nil {
				errs = append(errs, eerr)
			}
			return errors.Join(errs...)
		}
	default:
		return sdk.VerbResult{}, fmt.Errorf("toxiproxy has no verb %q", verb)
	}
	if err := call(ctx, work); err != nil {
		return sdk.VerbResult{}, fmt.Errorf("toxiproxy.%s on %q: %w", verb, name, err)
	}
	return sdk.VerbResult{Value: "ok"}, nil
}
