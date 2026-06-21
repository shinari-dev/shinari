// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

// projectTemplate is the scaffold emitted by `shinari new <dir>`: a complete,
// runnable, zero-infrastructure project. `all:` includes dotfiles (.gitignore).
//
//go:embed all:templates/project
var projectTemplate embed.FS

const projectTemplateRoot = "templates/project"

func newNewCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "new <dir>",
		Short: "scaffold a new, runnable project into <dir>",
		Long: "Scaffold a complete, runnable project into <dir>. The generated project " +
			"drives a toy job store through exec with zero infrastructure, so " +
			"`shinari -p <dir> run` is green on the first try.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := scaffoldProject(args[0], stdout, stderr); code != 0 {
				return &exitError{code}
			}
			return nil
		},
	}
}

// scaffoldProject writes the embedded project template into dir. It refuses to
// clobber: if dir already holds a project.yml, or any individual target file
// already exists, it writes nothing and exits EX_USAGE.
func scaffoldProject(dir string, stdout, stderr io.Writer) int {
	if _, err := os.Stat(filepath.Join(dir, "project.yml")); err == nil {
		fmt.Fprintf(stderr, "%s already contains a project.yml — refusing to overwrite\n", dir)
		return exitUsage
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	data := struct{ Name string }{Name: filepath.Base(abs)}

	// Plan every file first, then check for collisions, then write — so a
	// partial scaffold never lands on top of existing files.
	type outFile struct {
		path    string
		content []byte
		mode    os.FileMode
	}
	var files []outFile

	err = fs.WalkDir(projectTemplate, projectTemplateRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(path, projectTemplateRoot+"/")
		raw, err := projectTemplate.ReadFile(path)
		if err != nil {
			return err
		}
		content := raw
		if strings.HasSuffix(rel, ".tmpl") {
			rel = strings.TrimSuffix(rel, ".tmpl")
			tmpl, err := template.New(rel).Parse(string(raw))
			if err != nil {
				return err
			}
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, data); err != nil {
				return err
			}
			content = buf.Bytes()
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(rel, ".sh") {
			mode = 0o755
		}
		files = append(files, outFile{path: filepath.Join(dir, rel), content: content, mode: mode})
		return nil
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	for _, f := range files {
		if _, err := os.Stat(f.path); err == nil {
			fmt.Fprintf(stderr, "%s already exists — refusing to overwrite\n", f.path)
			return exitUsage
		}
	}

	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := os.WriteFile(f.path, f.content, f.mode); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "scaffolded %s (%d files)\n\nNext:\n  shinari -p %s validate\n  shinari -p %s run\n",
		dir, len(files), dir, dir)
	return 0
}
