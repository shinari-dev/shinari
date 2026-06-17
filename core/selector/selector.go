// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package selector filters discovered scenarios by tag expression. It is
// shared by the engine (run) and the CLI (list) so both apply the same
// include/exclude semantics. Empty include matches all; empty exclude
// excludes none; exclude wins on conflict.
package selector

import (
	"fmt"

	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/core/tagexpr"
)

// Filter returns the scenarios that satisfy include and are not matched by
// exclude. A malformed expression is an error naming the offending flag.
func Filter(scenarios []*model.Scenario, include, exclude string) ([]*model.Scenario, error) {
	var inc, exc *tagexpr.Expr
	if include != "" {
		e, err := tagexpr.Compile(include)
		if err != nil {
			return nil, fmt.Errorf("--include-tags: %w", err)
		}
		inc = &e
	}
	if exclude != "" {
		e, err := tagexpr.Compile(exclude)
		if err != nil {
			return nil, fmt.Errorf("--exclude-tags: %w", err)
		}
		exc = &e
	}
	if inc == nil && exc == nil {
		return scenarios, nil
	}
	var out []*model.Scenario
	for _, sc := range scenarios {
		if inc != nil && !inc.Eval(sc.Tags) {
			continue
		}
		if exc != nil && exc.Eval(sc.Tags) {
			continue
		}
		out = append(out, sc)
	}
	return out, nil
}
