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
	var project, color string
	root := &cobra.Command{
		Use:           "shinari",
		Short:         "resilience integration testing",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVarP(&project, "project", "p", ".", "project directory (holds project.yml)")
	root.PersistentFlags().StringVar(&color, "color", "auto", "colorize output: auto, always, or never")
	root.AddCommand(
		newNewCmd(stdout, stderr),
		newInitCmd(&project, stdout, stderr),
		newValidateCmd(&project, &color, stdout, stderr, lookupEnv),
		newListCmd(&project, &color, stdout, stderr, lookupEnv),
		newExplainCmd(&project, stdout, stderr),
		newRunCmd(&project, &color, stdout, stderr, getenv, lookupEnv),
		newLogCmd(&project, &color, stdout, stderr, lookupEnv),
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
