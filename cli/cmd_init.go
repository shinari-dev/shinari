// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shinari-dev/shinari/core/registry"
)

func newInitCmd(project *string, stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "resolve providers, write shinari.lock.yml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := cmdInit(*project, stdout, stderr); code != 0 {
				return &exitError{code}
			}
			return nil
		},
	}
}

// cmdInit resolves every configured provider and writes the lock file.
// V1 has no registry fetch: built-ins pin the engine version, local
// composed providers pin a content checksum.
func cmdInit(dir string, stdout, stderr io.Writer) int {
	set, ok := load(dir, stderr)
	if !ok {
		return 1
	}
	if _, err := registry.New(set, set.Project.Providers, nil); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	type lockEntry struct {
		Kind     string `yaml:"kind"` // builtin | local
		Version  string `yaml:"version,omitempty"`
		Source   string `yaml:"source,omitempty"`
		Checksum string `yaml:"checksum,omitempty"`
	}
	lock := struct {
		Version   int                  `yaml:"version"`
		Providers map[string]lockEntry `yaml:"providers"`
	}{Version: 1, Providers: map[string]lockEntry{}}

	for name, cfg := range set.Project.Providers {
		if cfg.Use != "" {
			def, err := set.FindLocalProvider(cfg.Use)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			data, err := os.ReadFile(def.File)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			lock.Providers[name] = lockEntry{
				Kind: "local", Source: cfg.Use,
				Checksum: fmt.Sprintf("sha256:%x", sha256.Sum256(data)),
			}
			continue
		}
		lock.Providers[name] = lockEntry{Kind: "builtin", Version: version}
	}
	data, err := yaml.Marshal(lock)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	lockPath := filepath.Join(set.Project.Dir, "shinari.lock.yml")
	if err := os.WriteFile(lockPath, data, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s (%d provider(s))\n", lockPath, len(lock.Providers))
	return 0
}
