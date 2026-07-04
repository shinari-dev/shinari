// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// loadDotenv reads a .env file into a name→value map with the standard
// github.com/joho/godotenv parser (docker/compose-style: comments, quoting,
// escapes, and same-file ${VAR} expansion). A missing file is not an error: it
// returns (nil, false, nil), so an absent default .env is a no-op. The bool
// reports whether the file existed, letting the caller distinguish "no default
// .env" (fine) from "the --env-file you named does not exist" (an error the
// caller raises).
func loadDotenv(path string) (map[string]string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	// Strip a leading UTF-8 BOM (left by some Windows editors) that godotenv
	// would otherwise fold into the first variable name and reject.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	vals, perr := godotenv.Parse(bytes.NewReader(data))
	if perr != nil {
		return nil, true, fmt.Errorf("%s: %w", path, perr)
	}
	return vals, true, nil
}

// dotenvOverlay loads the .env overlay for a run. With no --env-file, it reads
// the default <root>/.env when present; its absence is a silent no-op. When
// envFile is given explicitly, a missing file is an error (matching compose,
// where naming a file you can't read is a mistake, not a fallback to none).
func dotenvOverlay(root, envFile string) (map[string]string, error) {
	path := envFile
	explicit := path != ""
	if !explicit {
		path = filepath.Join(root, ".env")
	}
	vals, existed, err := loadDotenv(path)
	if err != nil {
		return nil, err
	}
	if explicit && !existed {
		return nil, fmt.Errorf("--env-file %s: no such file", path)
	}
	return vals, nil
}

// layeredLookup composes a lookup where base (the process environment) wins over
// overlay (a .env file). It is handed to resolveEnv unchanged, so a value from
// either layer counts as "set": precedence is process env > .env > env: default.
// Only names the project's env: block declares are ever read, so undeclared keys
// in a .env file are ignored rather than injected.
func layeredLookup(base func(string) (string, bool), overlay map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		if v, ok := base(k); ok {
			return v, true
		}
		if v, ok := overlay[k]; ok {
			return v, true
		}
		return "", false
	}
}
