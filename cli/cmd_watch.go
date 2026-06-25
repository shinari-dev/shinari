// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/shinari-dev/shinari/cli/tui"
	"github.com/shinari-dev/shinari/core/engine"
)

func newWatchCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "watch <journal.jsonl>",
		Short: "replay a saved run journal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := cmdWatch(args[0], stdout, stderr); code != 0 {
				return &exitError{code}
			}
			return nil
		},
	}
}

func cmdWatch(path string, stdout, stderr io.Writer) int {
	events, err := tui.LoadJournal(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if isTerminal(stdout) {
		if err := tui.RunReplay(events); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	fmt.Fprint(stdout, tui.RenderRun(engine.Reduce(events), "shinari — replay", 0))
	return 0
}
