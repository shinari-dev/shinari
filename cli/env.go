// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"sort"
	"strings"
)

// resolveEnv resolves a project's declared env: block against the process
// environment. A set variable (present, even if empty) wins; otherwise the
// declared default is used; a declaration with a nil default that is unset is
// required and produces an error. Core never reads the environment — this runs
// in the CLI and the result is handed to engine.Options.Env.
func resolveEnv(decls map[string]any, lookup func(string) (string, bool)) (map[string]any, error) {
	out := make(map[string]any, len(decls))
	var missing []string
	for name, def := range decls {
		if v, ok := lookup(name); ok {
			out[name] = v
			continue
		}
		if def != nil {
			out[name] = def
			continue
		}
		missing = append(missing, name)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("required environment variable(s) unset: %s", strings.Join(missing, ", "))
	}
	return out, nil
}
