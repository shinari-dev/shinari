// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package discover walks a project tree and collects Shinari resources.
// Recognition is by header, not filename; no layout is imposed.
package discover

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/shinari-dev/shinari/core/model"
)

// Set is everything discovery found in a project.
type Set struct {
	Root      string
	Project   *model.Project
	Scenarios []*model.Scenario
	Providers []*model.ProviderDef
}

// Load walks dir, parses every .yml/.yaml, and collects resources by kind.
// A recognized header with a malformed body is an error, not a silent skip.
func Load(dir string) (*Set, error) {
	set := &Set{Root: dir}
	var errs []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".yml" && ext != ".yaml" {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		h, ok, herr := model.ParseHeader(data)
		if !ok {
			return nil // not a Shinari resource: silently ignored
		}
		if herr != nil {
			errs = append(errs, herr.Error()+" ("+path+")")
			return nil
		}
		switch h.Kind {
		case model.KindProject:
			p, perr := model.ParseProject(data, path)
			if perr != nil {
				errs = append(errs, perr.Error())
				return nil
			}
			if set.Project != nil {
				errs = append(errs, fmt.Sprintf("two kind: Project resources found: %s and %s — a project has exactly one root", set.Project.File, path))
				return nil
			}
			p.Dir = filepath.Dir(path)
			set.Project = p
		case model.KindScenario:
			sc, serr := model.ParseScenario(data, path)
			if serr != nil {
				errs = append(errs, serr.Error())
				return nil
			}
			sc.Suite = suiteOf(dir, path)
			set.Scenarios = append(set.Scenarios, sc)
		case model.KindProvider:
			pd, derr := model.ParseProviderDef(data, path)
			if derr != nil {
				errs = append(errs, derr.Error())
				return nil
			}
			set.Providers = append(set.Providers, pd)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return set, fmt.Errorf("discovery found %d invalid resource(s):\n  - %s", len(errs), strings.Join(errs, "\n  - "))
	}
	if set.Project == nil {
		return set, fmt.Errorf("no kind: Project resource found under %s — a project root is required", dir)
	}
	return set, nil
}

// suiteOf derives the organizational suite of a scenario: the first
// directory under scenarios/ when the file lives there, else the parent
// directory name, else "" for files at the root.
func suiteOf(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) >= 3 && parts[0] == "scenarios" {
		return parts[1]
	}
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return ""
}

// FindLocalProvider resolves a `use: ./providers/foo` reference against the
// loaded set: a path to a file or directory containing one ProviderDef.
func (s *Set) FindLocalProvider(use string) (*model.ProviderDef, error) {
	abs := use
	if !filepath.IsAbs(use) && s.Project != nil {
		abs = filepath.Join(s.Project.Dir, use)
	}
	for _, pd := range s.Providers {
		f := filepath.Clean(pd.File)
		if f == filepath.Clean(abs) ||
			strings.HasPrefix(f, filepath.Clean(abs)+string(filepath.Separator)) ||
			strings.TrimSuffix(f, filepath.Ext(f)) == filepath.Clean(abs) {
			return pd, nil
		}
	}
	return nil, fmt.Errorf("no kind: Provider resource found at %s", use)
}
