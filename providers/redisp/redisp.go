// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package redisp is the redis built-in provider: drive and observe a Redis
// cache from a scenario. It offers probes (ping, get, exists, info) to assert
// on and actions (set, del, cmd) to drive workload, over the go-redis client.
// The cache outage or latency itself is injected by the fault providers (net,
// toxiproxy, docker); redis is the workload and observation lens, the same way
// sql pairs with those faults.
package redisp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

type Provider struct {
	rdb *redis.Client
}

func init() { sdk.Register("redis", New) }

func New() sdk.Provider { return &Provider{} }

func (p *Provider) Type() string { return "redis" }

// Configure builds the client from either a url (redis://…) or an addr plus
// optional auth and db. redis.NewClient does not connect; the real connection
// is lazy on the first verb, after setup has brought the cache up — so like
// sql.Open / grpc.NewClient, Configure must not ping.
func (p *Provider) Configure(cfg map[string]any) error {
	if url, _ := cfg["url"].(string); url != "" {
		opts, err := redis.ParseURL(url)
		if err != nil {
			return fmt.Errorf("redis: parse url: %w", err)
		}
		p.rdb = redis.NewClient(opts)
		return nil
	}
	addr, _ := cfg["addr"].(string)
	if addr == "" {
		return fmt.Errorf("redis: addr or url is required")
	}
	opts := &redis.Options{Addr: addr}
	opts.Username, _ = cfg["username"].(string)
	opts.Password, _ = cfg["password"].(string)
	if db, ok := conv.ToFloat(cfg["db"]); ok {
		opts.DB = int(db)
	}
	p.rdb = redis.NewClient(opts)
	return nil
}

func (p *Provider) Verbs() []sdk.VerbSpec {
	return []sdk.VerbSpec{
		{Name: "ping", Kind: sdk.KindProbe, SideEffects: false},
		{Name: "get", Kind: sdk.KindProbe, SideEffects: false, Primary: "key",
			Args: []sdk.ArgSpec{{Name: "key", Type: "string", Required: true}}},
		{Name: "set", Kind: sdk.KindAction, SideEffects: true, Primary: "key",
			Args: []sdk.ArgSpec{
				{Name: "key", Type: "string", Required: true},
				{Name: "value", Type: "any", Required: true},
				{Name: "ex", Type: "number"},
			}},
		{Name: "del", Kind: sdk.KindAction, SideEffects: true, Primary: "keys",
			Args: []sdk.ArgSpec{{Name: "keys", Type: "list", Required: true}}},
		{Name: "exists", Kind: sdk.KindProbe, SideEffects: false, Primary: "keys",
			Args: []sdk.ArgSpec{{Name: "keys", Type: "list", Required: true}}},
		{Name: "info", Kind: sdk.KindProbe, SideEffects: false, Primary: "section",
			Args: []sdk.ArgSpec{{Name: "section", Type: "string"}}},
		{Name: "cmd", Kind: sdk.KindAction, SideEffects: true, Primary: "args",
			Args: []sdk.ArgSpec{{Name: "args", Type: "list", Required: true}}},
	}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	switch verb {
	case "ping":
		if err := p.rdb.Ping(ctx).Err(); err != nil {
			return sdk.VerbResult{}, fmt.Errorf("redis.ping: %w", err)
		}
		return sdk.VerbResult{Value: true, Output: "PONG"}, nil

	case "get":
		key, _ := args["key"].(string)
		v, err := p.rdb.Get(ctx, key).Result()
		if err == redis.Nil {
			// A cache miss is a normal observation, not a failure: scenarios
			// assert a key is absent (e.g. gone after an outage).
			return sdk.VerbResult{Value: nil, Output: "(nil)", Meta: map[string]any{"hit": false}}, nil
		}
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("redis.get %s: %w", key, err)
		}
		return sdk.VerbResult{Value: v, Output: v, Meta: map[string]any{"hit": true}}, nil

	case "set":
		key, _ := args["key"].(string)
		var ttl time.Duration
		if ex, ok := conv.ToFloat(args["ex"]); ok && ex > 0 {
			ttl = time.Duration(ex * float64(time.Second))
		}
		if err := p.rdb.Set(ctx, key, conv.ToString(args["value"]), ttl).Err(); err != nil {
			return sdk.VerbResult{}, fmt.Errorf("redis.set %s: %w", key, err)
		}
		return sdk.VerbResult{Value: true, Output: "OK"}, nil

	case "del":
		keys := strKeys(args)
		n, err := p.rdb.Del(ctx, keys...).Result()
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("redis.del %v: %w", keys, err)
		}
		return sdk.VerbResult{Value: n, Output: fmt.Sprintf("deleted %d", n), Meta: map[string]any{"deleted": n}}, nil

	case "exists":
		keys := strKeys(args)
		n, err := p.rdb.Exists(ctx, keys...).Result()
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("redis.exists %v: %w", keys, err)
		}
		return sdk.VerbResult{Value: n, Output: fmt.Sprintf("%d present", n)}, nil

	case "info":
		section, _ := args["section"].(string)
		var sections []string
		if section != "" {
			sections = []string{section}
		}
		text, err := p.rdb.Info(ctx, sections...).Result()
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("redis.info: %w", err)
		}
		return sdk.VerbResult{Value: parseInfo(text), Output: text}, nil

	case "cmd":
		cmd := listOf(args, "args")
		if len(cmd) == 0 {
			return sdk.VerbResult{}, fmt.Errorf("redis.cmd: args is required (e.g. [\"INCR\", \"counter\"])")
		}
		v, err := p.rdb.Do(ctx, cmd...).Result()
		if err != nil && err != redis.Nil {
			return sdk.VerbResult{}, fmt.Errorf("redis.cmd %v: %w", cmd, err)
		}
		v = normalize(v)
		return sdk.VerbResult{Value: v, Output: conv.ToString(v)}, nil

	default:
		return sdk.VerbResult{}, fmt.Errorf("redis has no verb %q", verb)
	}
}

// listOf reads a list arg under name, accepting either a YAML list or a single
// scalar so `with: mykey` and `with: [a, b]` both work for del/exists/cmd.
func listOf(args map[string]any, name string) []any {
	switch v := args[name].(type) {
	case []any:
		return v
	case nil:
		return nil
	default:
		return []any{v}
	}
}

// strKeys reads the `keys` arg as strings for del/exists.
func strKeys(args map[string]any) []string {
	raw := listOf(args, "keys")
	keys := make([]string, len(raw))
	for i, k := range raw {
		keys[i] = conv.ToString(k)
	}
	return keys
}

// normalize coerces client-returned values into types interpolation and assert
// understand: bulk replies can arrive as []byte.
func normalize(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// parseInfo turns Redis INFO output (CRLF-separated field:value lines, with
// "# Section" headers) into a flat field->value map so steps can read
// ".role", ".connected_clients", etc.
func parseInfo(text string) map[string]any {
	out := map[string]any{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			out[k] = v
		}
	}
	return out
}
