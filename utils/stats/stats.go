// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package stats reduces raw latency samples into the canonical window
// statistics shared by the `sample` builtin and the `load` provider. It is a
// dependency-free leaf: it imports only the standard library, so both core and
// providers may link it.
package stats

import "sort"

// Summarize reduces raw latency samples (milliseconds, any order) and an error
// count into the canonical observation map:
// { n, errors, errorRate, min, max, mean, p50, p95, p99 }. The input slice is
// copied, never mutated. n is the number of samples; errorRate is errors/n.
func Summarize(latsMs []float64, errors int) map[string]any {
	sorted := append([]float64(nil), latsMs...)
	sort.Float64s(sorted)
	n := len(sorted)
	return map[string]any{
		"n":         float64(n),
		"errors":    float64(errors),
		"errorRate": ratio(errors, n),
		"min":       at(sorted, 0),
		"max":       at(sorted, n-1),
		"mean":      mean(sorted),
		"p50":       percentile(sorted, 50),
		"p95":       percentile(sorted, 95),
		"p99":       percentile(sorted, 99),
	}
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func at(xs []float64, i int) float64 {
	if i < 0 || i >= len(xs) {
		return 0
	}
	return xs[i]
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// percentile is the nearest-rank value of sorted xs at the p-th percentile.
func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p*len(sorted)+99)/100 - 1 // ceil(p/100 * n) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
