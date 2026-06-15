// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package builtins holds the scenario *language*: the unprefixed
// engine verbs (assert, sleep, wait_until, background, stop_background)
// — specs here, control-flow execution in the engine — plus the closed
// assert-operator set shared by assert and wait_until.
package builtins

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

// Operators is the closed assert-operator set.
var Operators = []string{
	"equals", "notEquals", "contains", "absent", "in", "matches",
	"gt", "lt", "gte", "lte", "between",
}

func isOperator(k string) bool {
	for _, op := range Operators {
		if k == op {
			return true
		}
	}
	return false
}

// Specs returns the language builtins as verb specs so the registry can
// resolve them uniformly. Their execution lives in the engine (they need
// the timeline: nested probes, goroutines), not in a provider.
func Specs() map[string]sdk.VerbSpec {
	ops := make([]sdk.ArgSpec, 0, len(Operators)+1)
	ops = append(ops, sdk.ArgSpec{Name: "of", Type: "any"})
	for _, op := range Operators {
		ops = append(ops, sdk.ArgSpec{Name: op, Type: "any"})
	}
	waitArgs := append([]sdk.ArgSpec{
		{Name: "probe", Type: "map", Required: true},
		{Name: "read", Type: "string"},
		{Name: "timeout", Type: "number", Required: true},
		{Name: "interval", Type: "number"},
	}, ops[1:]...)
	return map[string]sdk.VerbSpec{
		"assert": {Name: "assert", Kind: sdk.KindAssertion, Args: ops},
		"sleep": {Name: "sleep", Kind: sdk.KindAction, Primary: "seconds",
			Args: []sdk.ArgSpec{{Name: "seconds", Type: "number", Required: true}}},
		"wait_until": {Name: "wait_until", Kind: sdk.KindProbe, Args: waitArgs},
		"background": {Name: "background", Kind: sdk.KindAction, SideEffects: true,
			Args: []sdk.ArgSpec{
				{Name: "name", Type: "string", Required: true},
				{Name: "step", Type: "map", Required: true},
			}},
		"stop_background": {Name: "stop_background", Kind: sdk.KindAction, Primary: "name",
			Args: []sdk.ArgSpec{{Name: "name", Type: "string", Required: true}}},
		"sample": {Name: "sample", Kind: sdk.KindProbe, Args: []sdk.ArgSpec{
			{Name: "probe", Type: "map", Required: true},
			{Name: "count", Type: "number"},
			{Name: "duration", Type: "number"},
			{Name: "interval", Type: "number"},
		}},
	}
}

// ExtractOperator finds the single operator key in an assert/wait_until
// args map. Zero or several operator keys is an error.
func ExtractOperator(args map[string]any) (op string, operand any, err error) {
	for k, v := range args {
		if isOperator(k) {
			if op != "" {
				return "", nil, fmt.Errorf("exactly one assert operator allowed, found both %q and %q", op, k)
			}
			op, operand = k, v
		}
	}
	if op == "" {
		return "", nil, fmt.Errorf("missing assert operator (one of: %s)", strings.Join(Operators, ", "))
	}
	return op, operand, nil
}

// Check evaluates `of` against the operator. It returns pass/fail plus a
// human message for the failure case.
func Check(of any, op string, operand any) (bool, string, error) {
	switch op {
	case "equals":
		return eq(of, operand), failMsg(of, "==", operand), nil
	case "notEquals":
		return !eq(of, operand), failMsg(of, "!=", operand), nil
	case "contains":
		ok, err := contains(of, operand)
		return ok, failMsg(of, "contains", operand), err
	case "absent":
		ok, err := contains(of, operand)
		return !ok, failMsg(of, "does not contain", operand), err
	case "in":
		list, ok := operand.([]any)
		if !ok {
			return false, "", fmt.Errorf("'in' needs a list operand, got %T", operand)
		}
		for _, e := range list {
			if eq(of, e) {
				return true, "", nil
			}
		}
		return false, failMsg(of, "in", operand), nil
	case "matches":
		pat, ok := operand.(string)
		if !ok {
			return false, "", fmt.Errorf("'matches' needs a string regex, got %T", operand)
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return false, "", fmt.Errorf("'matches': invalid regex %q: %w", pat, err)
		}
		return re.MatchString(conv.ToString(of)), failMsg(of, "matches", operand), nil
	case "gt", "lt", "gte", "lte":
		a, aok := conv.ToFloat(of)
		b, bok := conv.ToFloat(operand)
		if !aok || !bok {
			return false, "", fmt.Errorf("'%s' needs numbers, got %v and %v", op, of, operand)
		}
		pass := map[string]bool{"gt": a > b, "lt": a < b, "gte": a >= b, "lte": a <= b}[op]
		return pass, failMsg(of, op, operand), nil
	case "between":
		bounds, ok := operand.([]any)
		if !ok || len(bounds) != 2 {
			return false, "", fmt.Errorf("'between' needs [min, max], got %v", operand)
		}
		a, aok := conv.ToFloat(of)
		lo, lok := conv.ToFloat(bounds[0])
		hi, hok := conv.ToFloat(bounds[1])
		if !aok || !lok || !hok {
			return false, "", fmt.Errorf("'between' needs numbers, got %v in %v", of, operand)
		}
		return a >= lo && a <= hi, failMsg(of, "between", operand), nil
	default:
		return false, "", fmt.Errorf("unknown assert operator %q", op)
	}
}

// eq applies the comparison coercion: numeric when both sides parse as numbers,
// else string comparison.
func eq(a, b any) bool {
	if an, aok := conv.ToFloat(a); aok {
		if bn, bok := conv.ToFloat(b); bok {
			return an == bn
		}
	}
	return conv.ToString(a) == conv.ToString(b)
}

func contains(of, operand any) (bool, error) {
	switch t := of.(type) {
	case string:
		return strings.Contains(t, conv.ToString(operand)), nil
	case []any:
		for _, e := range t {
			if eq(e, operand) {
				return true, nil
			}
		}
		return false, nil
	case nil:
		return false, nil
	default:
		return strings.Contains(conv.ToString(of), conv.ToString(operand)), nil
	}
}

func failMsg(of any, rel string, operand any) string {
	return fmt.Sprintf("expected %v %s %v", of, rel, operand)
}
