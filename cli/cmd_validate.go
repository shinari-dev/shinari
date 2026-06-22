// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/shinari-dev/shinari/core/validate"
)

func newValidateCmd(project, color *string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "static checks, no run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := cmdValidate(*project, *color, stdout, stderr, lookupEnv); code != 0 {
				return &exitError{code}
			}
			return nil
		},
	}
}

func cmdValidate(dir, color string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	set, ok := load(dir, stderr)
	if !ok {
		return 1
	}
	pal := paletteFor(color, stdout, lookupEnv)
	findings := validate.Validate(set)
	errors := 0
	for _, f := range findings {
		line := f.String()
		if f.Severity == validate.Error {
			errors++
			line = pal.Fail(line)
		} else {
			line = pal.Warn(line)
		}
		fmt.Fprintln(stdout, line)
	}
	if errors > 0 {
		fmt.Fprintf(stdout, "\n%s\n", pal.Fail(fmt.Sprintf("%d error(s), %d warning(s)", errors, len(findings)-errors)))
		return 1
	}
	fmt.Fprintf(stdout, "%s — %d scenario(s), %d warning(s)\n", pal.Pass("valid"), len(set.Scenarios), len(findings))
	return 0
}
