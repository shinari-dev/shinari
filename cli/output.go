// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	"github.com/shinari-dev/shinari/core/model"
)

// reportFile maps an exporter key to its on-disk filename. The order is the
// order files are written and listed in the run summary.
type reportFile struct {
	Key  string
	Name string
}

var reportFiles = []reportFile{
	{"tsv", "results.tsv"},
	{"json", "results.json"},
	{"junit", "junit.xml"},
	{"journal", "journal.jsonl"},
	{"findings", "findings.md"},
	{"sarif", "findings.sarif"},
	{"html", "report.html"},
}

type otlpPlan struct {
	Enabled  bool
	Endpoint string
}

// outputPlan is the resolved decision for one run: where reports go, which file
// exporters write, and whether/where to export OTLP traces.
type outputPlan struct {
	Dir   string
	Files map[string]bool
	OTLP  otlpPlan
}

// resolveOutput applies defaults and CLI-flag precedence to the project's
// output: block. File exporters default on (explicit enabled: false disables);
// otlp defaults off. --out overrides output.dir; --otlp overrides the endpoint
// and forces export on. Errors if otlp ends up enabled with no endpoint or an
// unsupported protocol.
func resolveOutput(cfg model.OutputConfig, outFlag, otlpFlag string) (outputPlan, error) {
	p := outputPlan{Files: make(map[string]bool, len(reportFiles))}

	switch {
	case outFlag != "":
		p.Dir = outFlag
	case cfg.Dir != "":
		p.Dir = cfg.Dir
	default:
		p.Dir = "shinari-out"
	}

	for _, rf := range reportFiles {
		on := true
		if ec, ok := cfg.Exporters[rf.Key]; ok && ec.Enabled != nil {
			on = *ec.Enabled
		}
		p.Files[rf.Key] = on
	}

	otlp := cfg.Exporters["otlp"]
	endpoint := otlp.Endpoint
	enabled := otlp.Enabled != nil && *otlp.Enabled
	if otlpFlag != "" {
		endpoint = otlpFlag
		enabled = true
	}
	// A disabled exporter's config must not abort runs that never use it.
	if enabled {
		if otlp.Protocol != "" && otlp.Protocol != "grpc" {
			return p, fmt.Errorf("output: otlp protocol %q is not supported (only grpc)", otlp.Protocol)
		}
		if endpoint == "" {
			return p, fmt.Errorf("output: otlp is enabled but no endpoint is set")
		}
	}
	p.OTLP = otlpPlan{Enabled: enabled, Endpoint: endpoint}

	return p, nil
}
