// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Line-based YAML highlighting for the source view: a display aid, not a parser.
var (
	reYAMLComment = regexp.MustCompile(`^(\s*)#`)
	reYAMLKey     = regexp.MustCompile(`^(\s*(?:- )?)([^:\s][^:]*)(:)(.*)$`)
	reYAMLNum     = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
)

func highlightYAML(src string) string {
	keyS := lipgloss.NewStyle().Foreground(pass)
	scalarS := lipgloss.NewStyle().Foreground(steel)
	litS := lipgloss.NewStyle().Foreground(warn)
	dimS := lipgloss.NewStyle().Foreground(fgDim)

	lines := strings.Split(src, "\n")
	for i, line := range lines {
		switch {
		case reYAMLComment.MatchString(line):
			lines[i] = dimS.Render(line)
		default:
			if m := reYAMLKey.FindStringSubmatch(line); m != nil {
				lines[i] = dimS.Render(m[1]) + keyS.Render(m[2]) + dimS.Render(m[3]) +
					highlightYAMLValue(m[4], scalarS, litS, dimS)
			} else {
				lines[i] = highlightYAMLValue(line, scalarS, litS, dimS)
			}
		}
	}
	return strings.Join(lines, "\n")
}

// highlightYAMLValue colors a value region: scalar vs literal (number/bool/null)
// plus any trailing ` # comment`.
func highlightYAMLValue(s string, scalarS, litS, dimS lipgloss.Style) string {
	val, comment := s, ""
	if i := yamlCommentIndex(s); i >= 0 {
		val, comment = s[:i], s[i:]
	}
	trimmed := strings.TrimSpace(val)
	lead := val[:len(val)-len(strings.TrimLeft(val, " "))]
	trail := val[len(lead)+len(trimmed):] // trailing spaces, kept so a comment doesn't hug the value
	var rendered string
	switch {
	case trimmed == "":
		rendered = ""
	case reYAMLNum.MatchString(trimmed), trimmed == "true", trimmed == "false", trimmed == "null":
		rendered = litS.Render(trimmed)
	default:
		rendered = scalarS.Render(trimmed)
	}
	out := lead + rendered + trail
	if comment != "" {
		out += dimS.Render(comment)
	}
	return out
}

// yamlCommentIndex finds a ` #` comment start (a '#' at line start or after a
// space), which YAML requires; returns -1 if none.
func yamlCommentIndex(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '#' && (i == 0 || s[i-1] == ' ') {
			return i
		}
	}
	return -1
}
