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
	"unicode/utf8"
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

// Normalize coerces a driver/client-returned value into a type interpolation
// and assert understand: text columns and bulk replies can arrive as []byte.
func Normalize(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// Truncate caps s at n bytes without cutting inside a multibyte rune,
// appending an ellipsis when it had to cut — for bounded error/diagnostic
// output that stays valid UTF-8 in JSON reports and journals.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "..."
}

// BaseURL returns a provider's configured base URL: the first non-empty value
// among the "baseUrl" and "apiBase" config keys, with any trailing slash
// trimmed. Returns "" when neither is set. Shared by the HTTP-shaped providers
// so they agree on the config seam.
func BaseURL(cfg map[string]any) string {
	for _, key := range []string{"baseUrl", "apiBase"} {
		if v, ok := cfg[key].(string); ok && v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return ""
}

// JoinURL resolves a request reference against a provider base URL. An absolute
// ref (http:// or https://) is returned unchanged; otherwise, when base is set,
// ref is appended with exactly one separating slash. base is assumed already
// trimmed of a trailing slash (see BaseURL).
func JoinURL(base, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	if base == "" {
		return ref
	}
	return base + "/" + strings.TrimLeft(ref, "/")
}
