// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/shinari-dev/shinari/core/discover"
)

// newRootCmd assembles the command tree. stdout/stderr/getenv/lookupEnv are
// captured by each subcommand's RunE so the whole tree is testable through run().
func newRootCmd(stdout, stderr io.Writer, getenv func(string) string, lookupEnv func(string) (string, bool)) *cobra.Command {
	var project string
	root := &cobra.Command{
		Use:           "shinari",
		Short:         "resilience integration testing",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVarP(&project, "project", "p", ".", "project directory (holds project.yml)")
	root.AddCommand(
		newNewCmd(stdout, stderr),
		newInitCmd(&project, stdout, stderr),
		newValidateCmd(&project, stdout, stderr),
		newListCmd(&project, stdout, stderr),
		newRunCmd(&project, stdout, stderr, getenv, lookupEnv),
	)
	return root
}

func load(dir string, stderr io.Writer) (*discover.Set, bool) {
	set, err := discover.Load(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return set, false
	}
	return set, true
}
