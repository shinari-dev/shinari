// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/shinari-dev/shinari/core/validate"
)

func newValidateCmd(project *string, stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "static checks, no run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := cmdValidate(*project, stdout, stderr); code != 0 {
				return &exitError{code}
			}
			return nil
		},
	}
}

func cmdValidate(dir string, stdout, stderr io.Writer) int {
	set, ok := load(dir, stderr)
	if !ok {
		return 1
	}
	findings := validate.Validate(set)
	errors := 0
	for _, f := range findings {
		fmt.Fprintln(stdout, f.String())
		if f.Severity == validate.Error {
			errors++
		}
	}
	if errors > 0 {
		fmt.Fprintf(stdout, "\n%d error(s), %d warning(s)\n", errors, len(findings)-errors)
		return 1
	}
	fmt.Fprintf(stdout, "valid — %d scenario(s), %d warning(s)\n", len(set.Scenarios), len(findings))
	return 0
}
