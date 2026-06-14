// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package conv holds the small value helpers shared across the engine,
// builtins, interpolation and providers: coercing a decoded YAML/JSON
// value to a number or a string, and truncating text for diagnostics.
package conv

import (
	"fmt"
	"strconv"
	"strings"
)

// ToFloat coerces a decoded value to a float64. Numbers convert directly;
// strings parse (trimmed); everything else fails. ok=false leaves the
// caller to decide whether the absence is an error or a default.
func ToFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// ToString renders a value the way interpolation and comparisons embed it:
// numbers canonically (no trailing zeros, no exponent for whole floats),
// nil as empty, everything else via fmt.
func ToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

// Truncate caps s at n runes-worth of bytes, appending an ellipsis when it
// had to cut — for bounded error/diagnostic output.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
