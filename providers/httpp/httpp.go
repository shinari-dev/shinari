// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package httpp is the http built-in provider: request + capture a JSON
// field — the primitive composed domain providers build on.
package httpp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

type Provider struct {
	baseURL        string
	client         *http.Client
	defaultTimeout time.Duration // applied only when the caller passed no deadline
	basicUser      string        // project-level basic auth, applied to every request
	basicPass      string
}

func init() { sdk.Register("http", New) }

func New() sdk.Provider {
	// No client-level timeout: the request context governs, so a per-step
	// timeout: of any value is authoritative. Run() applies defaultTimeout
	// only when the caller passed no deadline.
	return &Provider{client: &http.Client{}, defaultTimeout: 30 * time.Second}
}

func (p *Provider) Type() string { return "http" }

func (p *Provider) Configure(cfg map[string]any) error {
	p.baseURL = conv.BaseURL(cfg)
	p.basicUser, p.basicPass = basicAuthOf(cfg["basicAuth"])
	return nil
}

// basicAuthOf extracts { username, password } from a basicAuth map (config
// default or per-step override). A missing/non-map value yields empty strings.
func basicAuthOf(v any) (user, pass string) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", ""
	}
	user, _ = m["username"].(string)
	pass, _ = m["password"].(string)
	return user, pass
}

func argSpecs() []sdk.ArgSpec {
	return []sdk.ArgSpec{
		{Name: "path", Type: "string", Required: true},
		{Name: "body", Type: "map"},
		{Name: "raw", Type: "string"},
		{Name: "contentType", Type: "string"},
		{Name: "form", Type: "map"},
		{Name: "headers", Type: "map"},
		{Name: "basicAuth", Type: "map"},
		{Name: "expectStatus", Type: "list"},
	}
}

// accepted reports whether status is in the expectStatus list, if any.
func accepted(spec any, status int) bool {
	list, ok := spec.([]any)
	if !ok {
		return false
	}
	for _, s := range list {
		if n, ok := conv.ToFloat(s); ok && int(n) == status {
			return true
		}
	}
	return false
}

func (p *Provider) Verbs() []sdk.VerbSpec {
	verbs := []sdk.VerbSpec{
		{Name: "get", Kind: sdk.KindProbe, SideEffects: false},
		{Name: "post", Kind: sdk.KindAction, SideEffects: true},
		{Name: "put", Kind: sdk.KindAction, SideEffects: true},
		{Name: "delete", Kind: sdk.KindAction, SideEffects: true},
	}
	for i := range verbs {
		verbs[i].Primary = "path"
		verbs[i].Args = argSpecs()
	}
	return verbs
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	if _, ok := ctx.Deadline(); !ok && p.defaultTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.defaultTimeout)
		defer cancel()
	}
	path, _ := args["path"].(string)
	full := conv.JoinURL(p.baseURL, path)

	var reqBody io.Reader
	contentType := ""
	if raw, ok := args["raw"].(string); ok && raw != "" {
		// A verbatim string body (raw YAML, NDJSON, plain text) — no JSON
		// marshalling. Pair with contentType: to label it.
		reqBody = strings.NewReader(raw)
		contentType = "text/plain; charset=utf-8"
	} else if body, ok := args["body"]; ok && body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("http.%s %s: encoding body: %w", verb, path, err)
		}
		reqBody = bytes.NewReader(data)
		contentType = "application/json"
	} else if form, ok := args["form"].(map[string]any); ok {
		vals := url.Values{}
		for k, v := range form {
			vals.Set(k, fmt.Sprintf("%v", v))
		}
		reqBody = strings.NewReader(vals.Encode())
		contentType = "application/x-www-form-urlencoded"
	}
	if ct, ok := args["contentType"].(string); ok && ct != "" {
		contentType = ct
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(verb), full, reqBody)
	if err != nil {
		return sdk.VerbResult{}, fmt.Errorf("http.%s %s: %w", verb, path, err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	// Basic auth: the project-level default, overridden per step by basicAuth:.
	// Set before headers: so an explicit Authorization: header still wins.
	if user, pass := p.basicUser, p.basicPass; user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	if user, pass := basicAuthOf(args["basicAuth"]); user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	if headers, ok := args["headers"].(map[string]any); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return sdk.VerbResult{}, fmt.Errorf("http.%s %s: %w", verb, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	meta := map[string]any{"status": resp.StatusCode, "bytes": len(raw)}

	var value any = string(raw)
	if strings.Contains(resp.Header.Get("Content-Type"), "json") {
		var decoded any
		if json.Unmarshal(raw, &decoded) == nil {
			value = decoded
		}
	}
	if accepted(args["expectStatus"], resp.StatusCode) || resp.StatusCode < 400 {
		return sdk.VerbResult{Value: value, Output: string(raw), Meta: meta}, nil
	}
	return sdk.VerbResult{Value: value, Output: string(raw), Meta: meta},
		fmt.Errorf("http.%s %s: status %d: %s", verb, path, resp.StatusCode, conv.Truncate(string(raw), 200))
}
