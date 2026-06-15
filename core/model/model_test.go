// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"strings"
	"testing"
)

// A reference scenario exercising every step-envelope feature.
const referenceScenario = `
apiVersion: shinari/v1
kind: Scenario
name: data-loss/worker-killed-mid-task
description: A long job survives a SIGKILL and completes on a peer, exactly once.

vars: { sleepSecs: 30 }

setup:
  - { run: docker.up, with: [postgres, app, worker-a] }
  - { run: app.ready }

steadyState:
  - { run: app.smoke }

method:
  - phase: "Submit a long job and confirm it is RUNNING on worker-a"
    steps:
      - { run: app.submit, with: { job: sleep, inputs: { seconds: "${.sleepSecs}" } }, as: job }
      - { run: app.await_state, with: { of: "${.job}", state: RUNNING, timeout: 30 }, desc: "job RUNNING before crash" }
  - phase: "SIGKILL worker-a; a peer recovers the job"
    steps:
      - { run: docker.up, with: [worker-b] }
      - { run: docker.kill, with: worker-a }
      - { run: sleep, with: 50 }

verify:
  - { run: app.await, with: { of: "${.job}", timeout: 420 } }
  - { run: app.succeeded, with: { of: "${.job}" }, desc: "job completed after the crash" }
  - { run: app.count, with: { job: sleep }, as: total }
  - { run: assert, with: { of: "${.total}", equals: 1 }, desc: "no duplicate job (exactly once)" }

teardown:
  - { run: toxiproxy.reset }
`

func TestParseSpecScenario(t *testing.T) {
	sc, err := ParseScenario([]byte(referenceScenario), "s.yml")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Name != "data-loss/worker-killed-mid-task" {
		t.Errorf("name = %q", sc.Name)
	}
	if len(sc.Setup) != 2 || len(sc.SteadyState) != 1 || len(sc.Method) != 2 || len(sc.Verify) != 4 || len(sc.Teardown) != 1 {
		t.Fatalf("section sizes: setup=%d steady=%d method=%d verify=%d teardown=%d",
			len(sc.Setup), len(sc.SteadyState), len(sc.Method), len(sc.Verify), len(sc.Teardown))
	}
	if !sc.HasTeardown {
		t.Error("HasTeardown should be true")
	}
	if sc.Method[0].Steps[0].As != "job" {
		t.Errorf("as: capture not parsed: %+v", sc.Method[0].Steps[0])
	}
	if sc.Method[1].Steps[1].Run != "docker.kill" {
		t.Errorf("step run = %q", sc.Method[1].Steps[1].Run)
	}
	if sc.Vars["sleepSecs"] != 30 {
		t.Errorf("vars = %v", sc.Vars)
	}
}

func TestNoTeardownMeansDefault(t *testing.T) {
	sc, err := ParseScenario([]byte("apiVersion: shinari/v1\nkind: Scenario\nname: x\nverify:\n  - { run: assert, with: { of: 1, equals: 1 } }\n"), "s.yml")
	if err != nil {
		t.Fatal(err)
	}
	if sc.HasTeardown {
		t.Error("absent teardown must report HasTeardown=false (default applies)")
	}
}

func TestUnknownStepKeyIsError(t *testing.T) {
	_, err := ParseScenario([]byte("apiVersion: shinari/v1\nkind: Scenario\nname: x\nsetup:\n  - { run: docker.up, withh: [a] }\n"), "s.yml")
	if err == nil || !strings.Contains(err.Error(), "withh") {
		t.Fatalf("want unknown-key error naming 'withh', got %v", err)
	}
}

func TestStepEffectValue(t *testing.T) {
	_, err := ParseScenario([]byte("apiVersion: shinari/v1\nkind: Scenario\nname: x\nmethod:\n  - phase: p\n    steps:\n      - { run: exec.run, with: \"tc ... loss 50%\", effect: outage }\n"), "s.yml")
	if err != nil {
		t.Fatalf("valid effect must parse, got %v", err)
	}
	_, err = ParseScenario([]byte("apiVersion: shinari/v1\nkind: Scenario\nname: x\nmethod:\n  - phase: p\n    steps:\n      - { run: exec.run, with: x, effect: outag }\n"), "s.yml")
	if err == nil || !strings.Contains(err.Error(), "effect") {
		t.Fatalf("want invalid-effect error, got %v", err)
	}
}

func TestStepMissingRunIsError(t *testing.T) {
	_, err := ParseScenario([]byte("apiVersion: shinari/v1\nkind: Scenario\nname: x\nsetup:\n  - { desc: nothing }\n"), "s.yml")
	if err == nil || !strings.Contains(err.Error(), "run") {
		t.Fatalf("want missing-run error, got %v", err)
	}
}

// A reference composed provider covering params, do, probe, and capture.
const referenceProvider = `
apiVersion: shinari/v1
kind: Provider
name: app
verbs:
  submit:
    # NOTE: writing [job, inputs?] unquoted is invalid YAML: a flow
    # scalar cannot end with '?]'. The optional marker must be quoted.
    params: [job, "inputs?"]
    do: [ { run: http.post, with: { path: "/jobs/${.job}", form: "${.inputs}" }, capture: { id: ".id" } } ]
  await:
    params: [of, timeout]
    do: [ { run: wait_until, with: { probe: { run: http.get, with: { path: "/jobs/${.of}" } },
            read: ".state", in: [SUCCESS,FAILED], timeout: "${.timeout}" } } ]
  count:
    params: [job]
    probe: { run: http.get, with: { path: "/jobs?type=${.job}" }, read: ".items | length" }
`

func TestParseSpecProvider(t *testing.T) {
	pd, err := ParseProviderDef([]byte(referenceProvider), "app.yml")
	if err != nil {
		t.Fatal(err)
	}
	if pd.Name != "app" {
		t.Errorf("name = %q", pd.Name)
	}
	names, opt := pd.Verbs["submit"].ParamNames()
	if len(names) != 2 || names[1] != "inputs" || !opt["inputs"] {
		t.Errorf("params = %v optional = %v", names, opt)
	}
	if pd.Verbs["count"].Probe == nil {
		t.Error("count must be a probe verb")
	}
	if pd.Verbs["count"].Probe.Read != ".items | length" {
		t.Errorf("read = %q", pd.Verbs["count"].Probe.Read)
	}
	if pd.Verbs["submit"].Do[0].Capture["id"] != ".id" {
		t.Errorf("capture = %v", pd.Verbs["submit"].Do[0].Capture)
	}
}

func TestProviderVerbNeedsBody(t *testing.T) {
	_, err := ParseProviderDef([]byte("apiVersion: shinari/v1\nkind: Provider\nname: p\nverbs:\n  empty: { params: [a] }\n"), "p.yml")
	if err == nil {
		t.Fatal("want error for verb with neither do nor probe")
	}
}

func TestHeaderRecognition(t *testing.T) {
	cases := []struct {
		yaml string
		ok   bool
	}{
		{"apiVersion: shinari/v1\nkind: Scenario\nname: x", true},
		{"apiVersion: shinari/v1\nkind: Project\nname: x", true},
		{"apiVersion: shinari/v2\nkind: Scenario\nname: x", false}, // unknown version
		{"apiVersion: shinari/v1\nkind: Widget\nname: x", false},   // unknown kind
		{"services:\n  app:\n    image: nginx", false},             // a compose file
		{"just a plain string", false},
	}
	for _, c := range cases {
		_, ok, _ := ParseHeader([]byte(c.yaml))
		if ok != c.ok {
			t.Errorf("Recognized(%q) = %v, want %v", c.yaml, ok, c.ok)
		}
	}
}

func TestRecognizedButNamelessIsError(t *testing.T) {
	_, ok, err := ParseHeader([]byte("apiVersion: shinari/v1\nkind: Scenario\n"))
	if !ok || err == nil {
		t.Fatalf("recognized header without name must be an error, got ok=%v err=%v", ok, err)
	}
}

func TestMergeProviders(t *testing.T) {
	base := map[string]ProviderConfig{
		"docker": {Config: map[string]any{"project": "a", "keep": true}},
	}
	over := map[string]ProviderConfig{
		"docker": {Config: map[string]any{"project": "b"}},
		"app":    {Use: "./providers/app"},
	}
	m := MergeProviders(base, over)
	if m["docker"].Config["project"] != "b" || m["docker"].Config["keep"] != true {
		t.Errorf("merge wrong: %v", m["docker"].Config)
	}
	if m["app"].Use != "./providers/app" {
		t.Errorf("added instance missing: %v", m)
	}
}
