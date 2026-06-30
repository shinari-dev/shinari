// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/shinari-dev/shinari/core/model"
)

func boolPtr(b bool) *bool { return &b }

func TestResolveOutputDefaults(t *testing.T) {
	p, err := resolveOutput(model.OutputConfig{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Dir != "shinari-out" {
		t.Fatalf("dir = %q, want shinari-out", p.Dir)
	}
	for _, k := range []string{"tsv", "json", "junit", "journal", "findings", "sarif"} {
		if !p.Files[k] {
			t.Errorf("exporter %q should default on", k)
		}
	}
	if p.OTLP.Enabled {
		t.Error("otlp should default off")
	}
}

func TestResolveOutputDisableOne(t *testing.T) {
	cfg := model.OutputConfig{Exporters: map[string]model.ExporterConfig{
		"junit": {Enabled: boolPtr(false)},
	}}
	p, err := resolveOutput(cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Files["junit"] {
		t.Error("junit explicitly disabled")
	}
	if !p.Files["sarif"] {
		t.Error("unlisted sarif must stay on (additive override, not whitelist)")
	}
}

func TestResolveOutputOTLPFromYAML(t *testing.T) {
	cfg := model.OutputConfig{Exporters: map[string]model.ExporterConfig{
		"otlp": {Enabled: boolPtr(true), Endpoint: "10.0.0.1:4317"},
	}}
	p, err := resolveOutput(cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !p.OTLP.Enabled || p.OTLP.Endpoint != "10.0.0.1:4317" {
		t.Fatalf("otlp = %+v", p.OTLP)
	}
}

func TestResolveOutputFlagsOverride(t *testing.T) {
	cfg := model.OutputConfig{
		Dir: "yaml-dir",
		Exporters: map[string]model.ExporterConfig{
			"otlp": {Enabled: boolPtr(true), Endpoint: "10.0.0.1:4317"},
		},
	}
	p, err := resolveOutput(cfg, "flag-dir", "127.0.0.1:9999")
	if err != nil {
		t.Fatal(err)
	}
	if p.Dir != "flag-dir" {
		t.Fatalf("--out must win: dir = %q", p.Dir)
	}
	if p.OTLP.Endpoint != "127.0.0.1:9999" {
		t.Fatalf("--otlp must win: endpoint = %q", p.OTLP.Endpoint)
	}
}

func TestResolveOutputFlagForcesOTLPOn(t *testing.T) {
	p, err := resolveOutput(model.OutputConfig{}, "", "127.0.0.1:4317")
	if err != nil {
		t.Fatal(err)
	}
	if !p.OTLP.Enabled || p.OTLP.Endpoint != "127.0.0.1:4317" {
		t.Fatalf("--otlp alone must enable export: %+v", p.OTLP)
	}
}

func TestResolveOutputEnabledNoEndpointIsError(t *testing.T) {
	cfg := model.OutputConfig{Exporters: map[string]model.ExporterConfig{
		"otlp": {Enabled: boolPtr(true)},
	}}
	if _, err := resolveOutput(cfg, "", ""); err == nil {
		t.Fatal("otlp enabled with no endpoint must error")
	}
}

func TestResolveOutputBadProtocolIsError(t *testing.T) {
	cfg := model.OutputConfig{Exporters: map[string]model.ExporterConfig{
		"otlp": {Enabled: boolPtr(true), Endpoint: "x:1", Protocol: "http"},
	}}
	if _, err := resolveOutput(cfg, "", ""); err == nil {
		t.Fatal("unsupported otlp protocol must error")
	}
}
