// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package stats_test

import (
	"testing"

	"github.com/shinari-dev/shinari/utils/stats"
)

func TestSummarizeBasic(t *testing.T) {
	got := stats.Summarize([]float64{10, 20, 30, 40}, 1)
	want := map[string]float64{
		"n": 4, "errors": 1, "errorRate": 0.25,
		"min": 10, "max": 40, "mean": 25,
		"p50": 20, "p95": 40, "p99": 40, // nearest-rank over 4 samples
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %v, want %v", k, got[k], w)
		}
	}
}

func TestSummarizeEmpty(t *testing.T) {
	got := stats.Summarize(nil, 0)
	for _, k := range []string{"n", "errors", "errorRate", "min", "max", "mean", "p50", "p95", "p99"} {
		if got[k] != float64(0) {
			t.Errorf("%s = %v, want 0", k, got[k])
		}
	}
}

func TestSummarizeSortsAndDoesNotMutate(t *testing.T) {
	in := []float64{30, 10, 20}
	got := stats.Summarize(in, 0)
	if got["min"] != float64(10) || got["max"] != float64(30) {
		t.Errorf("min/max = %v/%v, want 10/30", got["min"], got["max"])
	}
	if in[0] != 30 {
		t.Errorf("input was mutated: %v", in)
	}
}
