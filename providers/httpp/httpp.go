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
	baseURL string
	client  *http.Client
}

func init() { sdk.Register("http", New) }

func New() sdk.Provider {
	return &Provider{client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *Provider) Type() string { return "http" }

func (p *Provider) Configure(cfg map[string]any) error {
	for _, key := range []string{"baseUrl", "apiBase"} {
		if v, ok := cfg[key].(string); ok && v != "" {
			p.baseURL = strings.TrimRight(v, "/")
			return nil
		}
	}
	return nil
}

func argSpecs() []sdk.ArgSpec {
	return []sdk.ArgSpec{
		{Name: "path", Type: "string", Required: true},
		{Name: "body", Type: "map"},
		{Name: "form", Type: "map"},
		{Name: "headers", Type: "map"},
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
	path, _ := args["path"].(string)
	full := p.baseURL + path
	if !strings.HasPrefix(path, "/") && p.baseURL == "" {
		full = path // absolute URL given directly
	}

	var reqBody io.Reader
	contentType := ""
	if body, ok := args["body"]; ok && body != nil {
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

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(verb), full, reqBody)
	if err != nil {
		return sdk.VerbResult{}, fmt.Errorf("http.%s %s: %w", verb, path, err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
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
