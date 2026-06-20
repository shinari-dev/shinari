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

// nsRefRe matches a namespaced reference `.<ns>.<name>` (name optional): the
// same leading-boundary rule as rootRefRe, then a first path segment and an
// optional second. Best-effort, for static validation only.
var nsRefRe = regexp.MustCompile(`(^|[^.\w\])])\.([A-Za-z_]\w*)(?:\.([A-Za-z_]\w*))?`)

// Ref is a namespaced reference: `.vars.region` -> {Namespace:"vars", Name:"region"}.
type Ref struct {
	Namespace string
	Name      string
}

// NSRefs returns the distinct namespaced references a jq expression reads. A
// single-segment reference like `.foo` yields {Namespace:"foo", Name:""} so a
// caller can flag a reference that is not namespaced.
func NSRefs(expr string) []Ref {
	seen := map[string]bool{}
	var out []Ref
	for _, m := range nsRefRe.FindAllStringSubmatch(expr, -1) {
		r := Ref{Namespace: m[2], Name: m[3]}
		key := r.Namespace + "." + r.Name
		if !seen[key] {
			seen[key] = true
			out = append(out, r)
		}
	}
	return out
}

// Eval runs a jq expression against a value and returns the first result.
// The value must be JSON-shaped (maps, slices, scalars).
func Eval(expr string, value any) (any, error) {
	return EvalWith(expr, value, nil)
}

// EvalWith is Eval with named jq variables bound alongside the input document.
// It is how the engine exposes a probe result's metadata to read:/capture:/
// wait_until: the result value stays the `.` input (so `.id` keeps working)
// while facts like the HTTP status arrive as $meta (`$meta.status`). Keys must
// include the leading `$` (e.g. "$meta"); a nil/empty map behaves like Eval.
func EvalWith(expr string, value any, vars map[string]any) (any, error) {
	q, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression %q: %w", expr, err)
	}
	names := make([]string, 0, len(vars))
	vals := make([]any, 0, len(vars))
	for name, v := range vars {
		names = append(names, name)
		vals = append(vals, normalize(v))
	}
	code, err := gojq.Compile(q, gojq.WithVariables(names))
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression %q: %w", expr, err)
	}
	iter := code.Run(normalize(value), vals...)
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
