// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"

	"github.com/shinari-dev/shinari/cli/render"
)

// paletteFor decides whether console output is colored, from the --color mode
// ("auto", "always", "never"). In auto it honors the NO_COLOR convention and
// TERM=dumb, then falls back to detecting whether w is an interactive terminal.
func paletteFor(mode string, w io.Writer, lookupEnv func(string) (string, bool)) render.Palette {
	switch mode {
	case "always":
		return render.NewPalette(true)
	case "never":
		return render.NewPalette(false)
	default: // auto
		if _, ok := lookupEnv("NO_COLOR"); ok {
			return render.NewPalette(false)
		}
		if t, _ := lookupEnv("TERM"); t == "dumb" {
			return render.NewPalette(false)
		}
		f, ok := w.(*os.File)
		if !ok {
			return render.NewPalette(false)
		}
		return render.NewPalette(isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd()))
	}
}
