// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Command shinari (alias shi) is the v1 front end: it parses argv with Cobra,
// drives core, renders output, and maps verdicts to exit codes.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const version = "0.5.1"

// exitUsage is EX_USAGE: distinct from the verdict codes 0..3.
const exitUsage = 64

// exitError carries a specific process exit code out of a command's RunE so
// run can translate it back into the process status.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv, os.LookupEnv))
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string, lookupEnv func(string) (string, bool)) int {
	root := newRootCmd(stdout, stderr, getenv, lookupEnv)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if len(args) == 0 {
		_ = root.Usage()
		return exitUsage
	}
	err := root.Execute()
	if err == nil {
		return 0
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	// A Cobra parse / unknown-command error (errors are silenced on the root).
	fmt.Fprintln(stderr, err)
	return exitUsage
}
