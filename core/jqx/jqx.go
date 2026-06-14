// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package jqx wraps gojq so the spec's jq expressions (".id",
// ".items | length") work verbatim in read:/capture:.
package jqx

import (
	"fmt"

	"github.com/itchyny/gojq"
)

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
