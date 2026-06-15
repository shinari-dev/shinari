// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package jqx wraps gojq so the spec's jq expressions (".id",
// ".items | length") work verbatim in read:/capture:.
package jqx

import (
	"fmt"
	"regexp"

	"github.com/itchyny/gojq"
)

// rootRefRe matches a top-level input field access `.name`: a dot that is NOT
// preceded by another field, identifier, or closing bracket (which would make
// it a nested access like .a.b). Best-effort, for static validation only;
// dynamic keys (.["x"]) are not reported.
var rootRefRe = regexp.MustCompile(`(^|[^.\w\])])\.([A-Za-z_]\w*)`)

// RootRefs returns the distinct top-level input fields a jq expression reads,
// e.g. RootRefs(".rsp.value.total") == ["rsp"]. Used by validate to check a
// reference resolves to a known var or capture.
func RootRefs(expr string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range rootRefRe.FindAllStringSubmatch(expr, -1) {
		name := m[2]
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// Eval runs a jq expression against a value and returns the first result.
// The value must be JSON-shaped (maps, slices, scalars).
func Eval(expr string, value any) (any, error) {
	q, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression %q: %w", expr, err)
	}
	iter := q.Run(normalize(value))
	v, ok := iter.Next()
	if !ok {
		return nil, nil
	}
	if e, isErr := v.(error); isErr {
		return nil, fmt.Errorf("jq %q: %w", expr, e)
	}
	return v, nil
}

// normalize converts Go values gojq rejects (int, map[any]any from YAML)
// into JSON-shaped equivalents.
func normalize(v any) any {
	switch t := v.(type) {
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normalize(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = normalize(e)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[fmt.Sprintf("%v", k)] = normalize(e)
		}
		return out
	default:
		return v
	}
}
