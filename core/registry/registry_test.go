// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/sdk"

	// Blank import self-registers the "http" type, which the fixtures resolve.
	_ "github.com/shinari-dev/shinari/providers/httpp"
)

func init() { sdk.Register("fakedocker", fakeLifecycle) }

// fake is a scriptable native provider for tests.
type fake struct {
	typeName string
	verbs    []sdk.VerbSpec
	cfg      map[string]any
}

func (f *fake) Type() string                       { return f.typeName }
func (f *fake) Configure(cfg map[string]any) error { f.cfg = cfg; return nil }
func (f *fake) Verbs() []sdk.VerbSpec              { return f.verbs }
func (f *fake) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	return sdk.VerbResult{Value: "ok"}, nil
}

func fakeLifecycle() sdk.Provider {
	return &fake{typeName: "fakedocker", verbs: []sdk.VerbSpec{
		{Name: "up", Kind: sdk.KindAction, SideEffects: true, Primary: "services"},
		{Name: "down", Kind: sdk.KindAction, SideEffects: true},
		{Name: "kill", Kind: sdk.KindAction, SideEffects: true, Primary: "service"},
		{Name: "logs", Kind: sdk.KindProbe, Primary: "service"},
	}}
}

func loadSet(t *testing.T, files map[string]string) *discover.Set {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	set, err := discover.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

const appProvider = `apiVersion: shinari/v1
kind: Provider
name: app
verbs:
  submit:
    params: [job, "inputs?"]
    do: [ { run: http.post, with: { path: "/jobs/${.job}", form: "${.inputs}" }, capture: { id: ".id" } } ]
  count:
    params: [job]
    probe: { run: http.get, with: { path: "/jobs?type=${.job}" }, read: ".items | length" }
  check_count:
    params: [job]
    do:
      - { run: http.get, with: { path: "/jobs?type=${.job}" }, read: ".items | length", as: n }
      - { run: assert, with: { of: "${.n}", equals: 1 } }
`

func TestResolveNativeComposedAndBuiltin(t *testing.T) {
	set := loadSet(t, map[string]string{
		"project.yml":       "apiVersion: shinari/v1\nkind: Project\nname: p\n",
		"providers/app.yml": appProvider,
	})
	r, err := New(set, map[string]model.ProviderConfig{
		"http": {Config: map[string]any{"baseUrl": "http://localhost:1"}},
		"app":  {Use: "./providers/app"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if res, err := r.Resolve("http.get"); err != nil || res.Spec.Kind != sdk.KindProbe {
		t.Errorf("http.get: %+v %v", res, err)
	}
	if res, err := r.Resolve("app.submit"); err != nil || res.Composed == nil {
		t.Errorf("app.submit: %+v %v", res, err)
	}
	if res, err := r.Resolve("assert"); err != nil || res.Builtin != "assert" || res.Spec.Kind != sdk.KindAssertion {
		t.Errorf("assert: %+v %v", res, err)
	}
	if _, err := r.Resolve("nope.verb"); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("unknown instance must error naming it: %v", err)
	}
	if _, err := r.Resolve("http.teleport"); err == nil || !strings.Contains(err.Error(), "teleport") {
		t.Errorf("unknown verb must error naming it: %v", err)
	}
	if _, err := r.Resolve("unprefixed"); err == nil {
		t.Error("unknown builtin must error")
	}
}

func TestComposedKindInference(t *testing.T) {
	set := loadSet(t, map[string]string{
		"project.yml":       "apiVersion: shinari/v1\nkind: Project\nname: p\n",
		"providers/app.yml": appProvider,
	})
	r, err := New(set, map[string]model.ProviderConfig{
		"http": {},
		"app":  {Use: "./providers/app"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]sdk.Kind{
		"app.submit":      sdk.KindAction,    // mutating leaf (http.post)
		"app.count":       sdk.KindProbe,     // pure observation
		"app.check_count": sdk.KindAssertion, // assert leaf, no mutation
	}
	for run, want := range cases {
		res, err := r.Resolve(run)
		if err != nil {
			t.Fatal(err)
		}
		if res.Spec.Kind != want {
			t.Errorf("%s kind = %s, want %s", run, res.Spec.Kind, want)
		}
	}
}

func TestMacroNestingBounded(t *testing.T) {
	set := loadSet(t, map[string]string{
		"project.yml": "apiVersion: shinari/v1\nkind: Project\nname: p\n",
		"providers/a.yml": `apiVersion: shinari/v1
kind: Provider
name: a
verbs:
  one: { do: [ { run: b.two } ] }
`,
		"providers/b.yml": `apiVersion: shinari/v1
kind: Provider
name: b
verbs:
  two: { do: [ { run: c.three } ] }
`,
		"providers/c.yml": `apiVersion: shinari/v1
kind: Provider
name: c
verbs:
  three: { do: [ { run: http.get, with: { path: / } } ] }
`,
	})
	_, err := New(set, map[string]model.ProviderConfig{
		"http": {},
		"a":    {Use: "./providers/a"},
		"b":    {Use: "./providers/b"},
		"c":    {Use: "./providers/c"},
	})
	if err == nil || !strings.Contains(err.Error(), "one level") {
		t.Fatalf("want nesting-depth error, got %v", err)
	}
}

func TestNamedInstances(t *testing.T) {
	set := loadSet(t, map[string]string{"project.yml": "apiVersion: shinari/v1\nkind: Project\nname: p\n"})
	r, err := New(set, map[string]model.ProviderConfig{
		"dockA": {Source: "fakedocker"},
		"dockB": {Source: "fakedocker"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve("dockA.kill"); err != nil {
		t.Error(err)
	}
	if _, err := r.Resolve("dockB.kill"); err != nil {
		t.Error(err)
	}
	if lc := r.Lifecycle(); len(lc) != 2 {
		t.Errorf("lifecycle = %v", lc)
	}
}

func TestUnknownTypeError(t *testing.T) {
	set := loadSet(t, map[string]string{"project.yml": "apiVersion: shinari/v1\nkind: Project\nname: p\n"})
	_, err := New(set, map[string]model.ProviderConfig{"warp": {}})
	if err == nil || !strings.Contains(err.Error(), "warp") {
		t.Fatalf("want unknown-type error, got %v", err)
	}
}

func TestBindArgs(t *testing.T) {
	spec := sdk.VerbSpec{Name: "kill", Primary: "service",
		Args: []sdk.ArgSpec{{Name: "service", Type: "string", Required: true}, {Name: "signal", Type: "string"}}}

	if args, err := BindArgs(spec, "worker-a"); err != nil || args["service"] != "worker-a" {
		t.Errorf("scalar shorthand: %v %v", args, err)
	}
	if args, err := BindArgs(spec, map[string]any{"service": "w", "signal": "KILL"}); err != nil || args["signal"] != "KILL" {
		t.Errorf("map form: %v %v", args, err)
	}
	if _, err := BindArgs(spec, map[string]any{"sevrice": "w"}); err == nil || !strings.Contains(err.Error(), "sevrice") {
		t.Errorf("unknown arg must error: %v", err)
	}
	if _, err := BindArgs(spec, map[string]any{"signal": "KILL"}); err == nil || !strings.Contains(err.Error(), "service") {
		t.Errorf("missing required must error: %v", err)
	}
	listSpec := sdk.VerbSpec{Name: "up", Primary: "services", Args: []sdk.ArgSpec{{Name: "services", Type: "list"}}}
	if args, err := BindArgs(listSpec, []any{"a", "b"}); err != nil || len(args["services"].([]any)) != 2 {
		t.Errorf("list shorthand: %v %v", args, err)
	}
}
