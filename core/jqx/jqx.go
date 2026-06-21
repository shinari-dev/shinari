// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package jqx wraps gojq, the single expression language across Shinari. jq is
// chosen not for power Shinari needs everywhere but because it is a standard
// users already know that degrades gracefully across three tiers without
// Shinari ever owning a parser:
//
//   - navigation, for ${...} interpolation and with: references
//     (.vars.job, .outputs.rsp.value.p95);
//   - predicates, for the when: guard (.outputs.total.value > 1, .vars.n > 0);
//   - transforms, for read:/capture: ([.runs[] | select(.failed)] | length,
//     .env.PORT // 8080).
//
// Comparisons and aggregation that could live in jq deliberately do not:
// assertions keep their predicate in an operator (gt:, contains:) for
// diagnostics and the findings ledger, and window statistics are computed in
// Go. So the language earns its keep at the navigation and guard tiers, with
// the transform tier as the escape hatch nobody has to learn until they need it.
package jqx

import (
	"fmt"

	"github.com/itchyny/gojq"
)

// Ref is a namespaced reference: `.vars.region` -> {Namespace:"vars", Name:"region"}.
type Ref struct {
	Namespace string
	Name      string
}

// NSRefs returns the distinct namespaced references a jq expression reads from
// its root input, in source order. `.vars.region` yields {vars, region}; a
// single-segment `.foo` yields {foo, ""} so a caller can flag a reference that
// is not namespaced. It walks the parsed jq, so only field accesses that read
// the engine root document are reported: a field after a pipe (`.runs |
// length`) or inside a function argument (`map(.state)`) reads a rebound `.`,
// not the root, and is correctly left out. A parse error yields no refs; the
// evaluator reports it when the expression actually runs.
func NSRefs(expr string) []Ref {
	q, err := gojq.Parse(expr)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []Ref
	add := func(r Ref) {
		key := r.Namespace + "." + r.Name
		if !seen[key] {
			seen[key] = true
			out = append(out, r)
		}
	}
	var walkQuery func(q *gojq.Query, root bool)
	var walkTerm func(t *gojq.Term, root bool)
	walkQuery = func(q *gojq.Query, root bool) {
		if q == nil {
			return
		}
		if q.Term != nil {
			walkTerm(q.Term, root)
			return
		}
		// Binary form `Left <Op> Right`: every operator feeds both sides the
		// same input except pipe, which feeds the right side the left's output
		// (so a field on the right reads a rebound `.`, not the root). A
		// pattern binding (`… as $x | …`) is conservatively treated like pipe.
		walkQuery(q.Left, root)
		walkQuery(q.Right, root && q.Op != gojq.OpPipe && len(q.Patterns) == 0)
	}
	walkTerm = func(t *gojq.Term, root bool) {
		if t == nil || !root {
			return
		}
		switch t.Type {
		case gojq.TermTypeIndex, gojq.TermTypeIdentity:
			// Collect the static leading path, e.g. .outputs.rsp.value -> the
			// first two field names. Dynamic keys (.["x"], .[0]) and iterators
			// (.[]) contribute no name and stop the namespace/name pair early.
			var path []string
			if t.Type == gojq.TermTypeIndex && t.Index != nil && t.Index.Name != "" {
				path = append(path, t.Index.Name)
			}
			for _, s := range t.SuffixList {
				if s.Index == nil || s.Index.Name == "" {
					break
				}
				path = append(path, s.Index.Name)
			}
			if len(path) == 0 {
				return // bare identity `.`, or a dynamic leading index
			}
			r := Ref{Namespace: path[0]}
			if len(path) > 1 {
				r.Name = path[1]
			}
			add(r)
		case gojq.TermTypeQuery:
			walkQuery(t.Query, root) // parenthesized: same input as the term
		case gojq.TermTypeArray:
			if t.Array != nil {
				walkQuery(t.Array.Query, root) // [EXPR]: EXPR reads the same input
			}
		}
	}
	walkQuery(q, true)
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
