// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// scenarioTemplate returns a valid starter scenario. kind is "minimal" or
// "fault-inject"; both parse and produce no validate errors on creation.
func scenarioTemplate(name, kind string) string {
	head := fmt.Sprintf("apiVersion: shinari/v1\nkind: Scenario\nname: %s\ndescription: TODO describe what resilience property this asserts.\n\n", name)
	steady := "steadyState:\n  - run: assert\n    with: { of: \"${true}\", equals: true }\n    desc: \"system is up\"\n\n"
	verify := "verify:\n  - run: assert\n    with: { of: \"${true}\", equals: true }\n    desc: \"system recovered\"\n\n"
	setup := "setup:\n  - run: exec.run\n    with: \"true\"\n\n"
	teardown := "teardown:\n  - run: exec.run\n    with: \"true\"\n"
	if kind == "fault-inject" {
		method := "method:\n  - phase: \"Inject a fault and observe recovery\"\n    steps:\n" +
			"      - run: exec.run\n        with: \"true\"\n        effect: outage\n        desc: \"inject the fault here (replace with a real fault verb)\"\n" +
			"      - run: assert\n        with: { of: \"${true}\", equals: true }\n        finding: \"recovery is slower than the target\"\n\n"
		return head + setup + steady + method + verify + teardown
	}
	method := "method:\n  - phase: \"Inject a fault\"\n    steps:\n" +
		"      - run: assert\n        with: { of: \"${true}\", equals: true }\n        desc: \"placeholder method step\"\n\n"
	return head + setup + steady + method + verify + teardown
}

// scaffoldName is the shape a scaffolded scenario name or suite may take: it
// is interpolated raw into YAML and a filename, so quotes, colons, slashes,
// and whitespace are out.
var scaffoldName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// writeScenario writes a scaffolded scenario under scenarios/<suite>/<name>.yml
// and returns its path. It errors rather than overwrite an existing file.
func writeScenario(root, suite, name, kind string) (string, error) {
	if !scaffoldName.MatchString(name) {
		return "", fmt.Errorf("scenario name %q must match %s", name, scaffoldName)
	}
	if !scaffoldName.MatchString(suite) {
		return "", fmt.Errorf("suite %q must match %s", suite, scaffoldName)
	}
	dir := filepath.Join(root, "scenarios", suite)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".yml")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists", path)
	}
	if err := os.WriteFile(path, []byte(scenarioTemplate(name, kind)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
