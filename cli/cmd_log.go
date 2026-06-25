// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/shinari-dev/shinari/cli/history"
)

func newLogCmd(project, color *string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "finding trend across recorded runs (see run --record)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := cmdLog(*project, *color, stdout, stderr, lookupEnv); code != 0 {
				return &exitError{code}
			}
			return nil
		},
	}
	return cmd
}

func cmdLog(dir, color string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	set, ok := load(dir, stderr)
	if !ok {
		return 1
	}
	pal := paletteFor(color, stdout, lookupEnv)

	path := history.Path(set.Root)
	records, err := history.Load(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(records) == 0 {
		fmt.Fprintln(stdout, "no history recorded yet — run with --record to start one")
		return 0
	}

	fmt.Fprintf(stdout, "%d runs recorded\n\n", len(records))
	for _, t := range history.FoldTrend(records) {
		status := t.Status
		switch t.Status {
		case "open":
			status = pal.Finding(t.Status)
		case "fixed":
			status = pal.Pass(t.Status)
		}
		fmt.Fprintf(stdout, "  %-16s  %2d runs  %-6s  %s\n", t.ID, t.Runs, status, t.Narrative)
	}
	return 0
}
