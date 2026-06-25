// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

// isTerminal reports whether w is an interactive terminal. A bytes.Buffer (in
// tests) or a redirected file is not, so the TUI commands fall back to a
// static render instead of starting a program that needs a TTY.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd())
}
