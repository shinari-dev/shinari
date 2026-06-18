// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/core/selector"
)

func newListCmd(project *string, stdout, stderr io.Writer) *cobra.Command {
	var include, exclude string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "discovered scenarios, grouped by suite",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := cmdList(*project, include, exclude, stdout, stderr); code != 0 {
				return &exitError{code}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&include, "include-tags", "", "list only scenarios matching this tag expression")
	cmd.Flags().StringVar(&exclude, "exclude-tags", "", "exclude scenarios matching this tag expression")
	return cmd
}

func cmdList(dir, include, exclude string, stdout, stderr io.Writer) int {
	set, ok := load(dir, stderr)
	if !ok {
		return 1
	}
	scenarios, err := selector.Filter(set.Scenarios, include, exclude)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	bySuite := map[string][]*model.Scenario{}
	for _, sc := range scenarios {
		bySuite[sc.Suite] = append(bySuite[sc.Suite], sc)
	}
	suites := make([]string, 0, len(bySuite))
	for s := range bySuite {
		suites = append(suites, s)
	}
	sort.Strings(suites)
	for _, suite := range suites {
		name := suite
		if name == "" {
			name = "(no suite)"
		}
		fmt.Fprintf(stdout, "%s\n", name)
		scs := bySuite[suite]
		sort.Slice(scs, func(i, j int) bool { return scs[i].Name < scs[j].Name })
		for _, sc := range scs {
			fmt.Fprintf(stdout, "  %s", sc.Name)
			if sc.Description != "" {
				fmt.Fprintf(stdout, " — %s", sc.Description)
			}
			if len(sc.Tags) > 0 {
				fmt.Fprintf(stdout, " [%s]", strings.Join(sc.Tags, ", "))
			}
			fmt.Fprintln(stdout)
		}
	}
	return 0
}
