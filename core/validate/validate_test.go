// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package validate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/sdk"

	// Blank import self-registers "http", which several fixtures configure.
	_ "github.com/shinari-dev/shinari/providers/httpp"
)

// lifecycleFake gives tests a lifecycle provider without docker.
type lifecycleFake struct{}

func (lifecycleFake) Type() string                   { return "lcfake" }
func (lifecycleFake) Configure(map[string]any) error { return nil }
func (lifecycleFake) Verbs() []sdk.VerbSpec {
	return []sdk.VerbSpec{
		{Name: "up", Kind: sdk.KindAction, SideEffects: true, Primary: "services"},
		{Name: "down", Kind: sdk.KindAction, SideEffects: true},
		{Name: "kill", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "service",
			Args: []sdk.ArgSpec{{Name: "service", Type: "string", Required: true}}},
		{Name: "submit", Kind: sdk.KindAction, SideEffects: true, Primary: "job"},
		{Name: "await", Kind: sdk.KindProbe, Primary: "of",
			Args: []sdk.ArgSpec{{Name: "of", Type: "string", Required: true}}},
		{Name: "count", Kind: sdk.KindProbe, Primary: "job"},
	}
}
func (lifecycleFake) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	return sdk.VerbResult{Value: "ok"}, nil
}

func init() { sdk.Register("lcfake", func() sdk.Provider { return lifecycleFake{} }) }

func load(t *testing.T, files map[string]string) *discover.Set {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
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

const projectWithSut = `apiVersion: shinari/v1
kind: Project
name: p
providers:
  sut: { source: lcfake }
`

func findRule(fs []Finding, rule int) *Finding {
	for i := range fs {
		if fs[i].Rule == rule {
			return &fs[i]
		}
	}
	return nil
}

func TestCleanScenarioHasNoErrors(t *testing.T) {
	set := load(t, map[string]string{
		"project.yml": projectWithSut,
		"s.yml": `apiVersion: shinari/v1
kind: Scenario
name: clean
vars: { n: 1 }
setup:
  - { run: sut.up, with: [app] }
method:
  - phase: f
    steps:
      - { run: sut.submit, with: job, as: id }
      - { run: sut.kill, with: app }
verify:
  - { run: sut.await, with: "${id}" }
  - { run: sut.count, with: job, as: total }
  - { run: assert, with: { of: "${total}", equals: 1 } }
`,
	})
	for _, f := range Validate(set) {
		if f.Severity == Error {
			t.Errorf("unexpected error: %s", f)
		}
	}
}

func TestRule2WithSpecMismatch(t *testing.T) {
	set := load(t, map[string]string{
		"project.yml": projectWithSut,
		"s.yml": `apiVersion: shinari/v1
kind: Scenario
name: s
verify:
  - { run: sut.kill, with: { sevrice: app } }
`,
	})
	f := findRule(Validate(set), 2)
	if f == nil || f.Severity != Error {
		t.Fatalf("want rule 2 error, got %v", Validate(set))
	}
}

func TestRule2MissingAssertOperator(t *testing.T) {
	set := load(t, map[string]string{
		"project.yml": projectWithSut,
		"s.yml": `apiVersion: shinari/v1
kind: Scenario
name: s
verify:
  - { run: assert, with: { of: 1 } }
`,
	})
	if f := findRule(Validate(set), 2); f == nil {
		t.Fatal("want rule 2 error for missing operator")
	}
}

func TestRule3UnresolvableVerb(t *testing.T) {
	set := load(t, map[string]string{
		"project.yml": projectWithSut,
		"s.yml": `apiVersion: shinari/v1
kind: Scenario
name: s
verify:
  - { run: ghost.poke }
`,
	})
	if f := findRule(Validate(set), 3); f == nil {
		t.Fatal("want rule 3 error")
	}
}

func TestRule3OnAbsentSkipIsAllowed(t *testing.T) {
	set := load(t, map[string]string{
		"project.yml": projectWithSut,
		"s.yml": `apiVersion: shinari/v1
kind: Scenario
name: s
verify:
  - { run: ghost.poke, onAbsent: skip }
`,
	})
	if f := findRule(Validate(set), 3); f != nil {
		t.Fatalf("onAbsent: skip must not be a rule 3 error: %v", f)
	}
}

func TestRule4MacroNesting(t *testing.T) {
	set := load(t, map[string]string{
		"project.yml": `apiVersion: shinari/v1
kind: Project
name: p
providers:
  http: { config: { baseUrl: http://x } }
  a: { use: ./providers/a }
  b: { use: ./providers/b }
  c: { use: ./providers/c }
`,
		"providers/a.yml": "apiVersion: shinari/v1\nkind: Provider\nname: a\nverbs:\n  one: { do: [ { run: b.two } ] }\n",
		"providers/b.yml": "apiVersion: shinari/v1\nkind: Provider\nname: b\nverbs:\n  two: { do: [ { run: c.three } ] }\n",
		"providers/c.yml": "apiVersion: shinari/v1\nkind: Provider\nname: c\nverbs:\n  three: { do: [ { run: http.get, with: { path: / } } ] }\n",
		"s.yml":           "apiVersion: shinari/v1\nkind: Scenario\nname: s\nverify:\n  - { run: a.one }\n",
	})
	if f := findRule(Validate(set), 4); f == nil {
		t.Fatal("want rule 4 error")
	}
}

func TestRule5FindingOnNonAssertion(t *testing.T) {
	set := load(t, map[string]string{
		"project.yml": projectWithSut,
		"s.yml": `apiVersion: shinari/v1
kind: Scenario
name: s
verify:
  - { run: sut.kill, with: app, finding: "not an assertion" }
`,
	})
	if f := findRule(Validate(set), 5); f == nil {
		t.Fatal("want rule 5 error")
	}
}

func TestRule6BackgroundCaptureBeforeStop(t *testing.T) {
	set := load(t, map[string]string{
		"project.yml": projectWithSut,
		"s.yml": `apiVersion: shinari/v1
kind: Scenario
name: s
method:
  - phase: p
    steps:
      - { run: background, with: { name: load, step: { run: sut.count, with: x } } }
      - { run: assert, with: { of: "${load}", equals: 1 } }
      - { run: stop_background, with: load, as: load }
`,
	})
	if f := findRule(Validate(set), 6); f == nil {
		t.Fatalf("want rule 6 error, got %v", Validate(set))
	}
}

func TestRule7RecoveryInvariant(t *testing.T) {
	scenario := `apiVersion: shinari/v1
kind: Scenario
name: s
method:
  - phase: p
    steps:
      - { run: sut.submit, with: job, as: id }
      - { run: sut.kill, with: app }
verify:
  - { run: sut.await, with: "${id}" }
`
	set := load(t, map[string]string{"project.yml": projectWithSut, "s.yml": scenario})
	f := findRule(Validate(set), 7)
	if f == nil || f.Severity != Error {
		t.Fatalf("recovery-shaped without exactly-once must be rule 7 error, got %v", Validate(set))
	}

	// satisfied by a finding:
	withFinding := scenario + `  - { run: sut.count, with: job, as: n }
  - { run: assert, with: { of: "${n}", equals: 1 }, finding: "dupes happen" }
`
	set2 := load(t, map[string]string{"project.yml": projectWithSut, "s2.yml": withFinding})
	if f := findRule(Validate(set2), 7); f != nil {
		t.Fatalf("finding: must satisfy rule 7: %v", f)
	}
}

func TestRule7DetectsComposedFault(t *testing.T) {
	// A composed verb with a non-builtin name inherits its leaf's outage
	// effect, so rule 7 recognizes it as a fault with no hardcoded name list.
	project := `apiVersion: shinari/v1
kind: Project
name: p
providers:
  sut: { source: lcfake }
  chaos: { use: ./providers/chaos }
`
	chaos := `apiVersion: shinari/v1
kind: Provider
name: chaos
verbs:
  cripple: { do: [ { run: sut.kill, with: app } ] }
`
	scenario := `apiVersion: shinari/v1
kind: Scenario
name: s
method:
  - phase: p
    steps:
      - { run: sut.submit, with: job, as: id }
      - { run: chaos.cripple }
verify:
  - { run: sut.await, with: "${id}" }
`
	set := load(t, map[string]string{
		"project.yml":         project,
		"providers/chaos.yml": chaos,
		"s.yml":               scenario,
	})
	if f := findRule(Validate(set), 7); f == nil {
		t.Fatalf("composed outage fault must trigger rule 7, got %v", Validate(set))
	}
}

func TestRule7StepEffectOverride(t *testing.T) {
	// sut.submit is EffectNone; a step-level effect: outage declares this
	// invocation a fault (the exec.run/http.post case, modeled on a fake).
	scenario := `apiVersion: shinari/v1
kind: Scenario
name: s
method:
  - phase: p
    steps:
      - { run: sut.submit, with: job, as: id }
      - { run: sut.submit, with: boom, effect: outage }
verify:
  - { run: sut.await, with: "${id}" }
`
	set := load(t, map[string]string{"project.yml": projectWithSut, "s.yml": scenario})
	if findRule(Validate(set), 7) == nil {
		t.Fatalf("step-level effect: outage must make the scenario recovery-shaped, got %v", Validate(set))
	}

	// Control: the same scenario without the override is not recovery-shaped.
	noFault := `apiVersion: shinari/v1
kind: Scenario
name: s
method:
  - phase: p
    steps:
      - { run: sut.submit, with: job, as: id }
verify:
  - { run: sut.await, with: "${id}" }
`
	set2 := load(t, map[string]string{"project.yml": projectWithSut, "s2.yml": noFault})
	if f := findRule(Validate(set2), 7); f != nil {
		t.Fatalf("no fault means no rule 7, got %v", f)
	}
}

func TestComposedFaultViaStepEffect(t *testing.T) {
	// A composed verb built on a polymorphic leaf inherits the leaf step's
	// declared effect, so the macro is recognized as a fault.
	project := `apiVersion: shinari/v1
kind: Project
name: p
providers:
  sut: { source: lcfake }
  chaos: { use: ./providers/chaos }
`
	chaos := `apiVersion: shinari/v1
kind: Provider
name: chaos
verbs:
  script_kill: { do: [ { run: sut.submit, with: boom, effect: outage } ] }
`
	scenario := `apiVersion: shinari/v1
kind: Scenario
name: s
method:
  - phase: p
    steps:
      - { run: sut.submit, with: job, as: id }
      - { run: chaos.script_kill }
verify:
  - { run: sut.await, with: "${id}" }
`
	set := load(t, map[string]string{
		"project.yml":         project,
		"providers/chaos.yml": chaos,
		"s.yml":               scenario,
	})
	if findRule(Validate(set), 7) == nil {
		t.Fatalf("composed verb inheriting a step-level outage must trigger rule 7, got %v", Validate(set))
	}
}

func TestRule8LifecycleCount(t *testing.T) {
	set := load(t, map[string]string{
		"project.yml": `apiVersion: shinari/v1
kind: Project
name: p
providers:
  sutA: { source: lcfake }
  sutB: { source: lcfake }
`,
		"s.yml": "apiVersion: shinari/v1\nkind: Scenario\nname: s\nverify:\n  - { run: assert, with: { of: 1, equals: 1 } }\n",
	})
	f := findRule(Validate(set), 8)
	if f == nil || f.Severity != Error {
		t.Fatalf("two lifecycle providers must be rule 8 error, got %v", Validate(set))
	}

	noLC := load(t, map[string]string{
		"project.yml": "apiVersion: shinari/v1\nkind: Project\nname: p\nproviders:\n  http: { config: { baseUrl: http://x } }\n",
		"s.yml":       "apiVersion: shinari/v1\nkind: Scenario\nname: s\nverify:\n  - { run: assert, with: { of: 1, equals: 1 } }\n",
	})
	f = findRule(Validate(noLC), 8)
	if f == nil || f.Severity != Warn {
		t.Fatalf("zero lifecycle providers must be rule 8 warn, got %v", Validate(noLC))
	}
}

func TestRule9SteadyStateMutation(t *testing.T) {
	set := load(t, map[string]string{
		"project.yml": projectWithSut,
		"s.yml": `apiVersion: shinari/v1
kind: Scenario
name: s
steadyState:
  - { run: sut.submit, with: job }
verify:
  - { run: assert, with: { of: 1, equals: 1 } }
`,
	})
	f := findRule(Validate(set), 9)
	if f == nil || f.Severity != Warn {
		t.Fatalf("want rule 9 warn, got %v", Validate(set))
	}
}

func TestRule10UnresolvedRef(t *testing.T) {
	set := load(t, map[string]string{
		"project.yml": projectWithSut,
		"s.yml": `apiVersion: shinari/v1
kind: Scenario
name: s
verify:
  - { run: sut.await, with: "${ghost}" }
`,
	})
	if f := findRule(Validate(set), 10); f == nil {
		t.Fatal("want rule 10 error")
	}
}

func TestRule10ComposedBodyRefs(t *testing.T) {
	set := load(t, map[string]string{
		"project.yml": `apiVersion: shinari/v1
kind: Project
name: p
providers:
  http: { config: { baseUrl: http://x } }
  app: { use: ./providers/app }
`,
		"providers/app.yml": `apiVersion: shinari/v1
kind: Provider
name: app
verbs:
  bad:
    params: [job]
    do: [ { run: http.get, with: { path: "/x/${nope}" } } ]
`,
		"s.yml": "apiVersion: shinari/v1\nkind: Scenario\nname: s\nverify:\n  - { run: assert, with: { of: 1, equals: 1 } }\n",
	})
	if f := findRule(Validate(set), 10); f == nil {
		t.Fatal("want rule 10 error for composed body ref")
	}
}
