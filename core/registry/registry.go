// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package registry holds the configured provider instances of a run and
// resolves every step's run: against the union of their verb specs.
package registry

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/shinari-dev/shinari/core/builtins"
	"github.com/shinari-dev/shinari/core/discover"
	"github.com/shinari-dev/shinari/core/model"
	"github.com/shinari-dev/shinari/sdk"
)

// VerbName returns the bare verb of a run: value, stripping the leading
// <instance>. namespace when present (language builtins have none).
func VerbName(run string) string {
	if i := strings.Index(run, "."); i >= 0 {
		return run[i+1:]
	}
	return run
}

// Instance is one configured provider: its name is the verb namespace.
type Instance struct {
	Name   string
	Native sdk.Provider       // nil when composed
	Def    *model.ProviderDef // nil when native
	specs  map[string]sdk.VerbSpec
}

// Resolution is what a step's run: binds to.
type Resolution struct {
	// Builtin is the language verb name when run: is unprefixed.
	Builtin  string
	Instance *Instance
	Spec     sdk.VerbSpec
	Composed *model.ComposedVerb // set when the verb is a YAML macro
}

type Registry struct {
	instances map[string]*Instance
	builtins  map[string]sdk.VerbSpec
}

// New configures every instance of a merged providers: block. Composed
// providers are looked up in the discovery set via use:. env is the run's
// resolved project environment (the declared env: block, resolved by the CLI);
// it is injected into each native instance's config so providers that shell out
// forward it to their subprocesses. Static callers (validate, explain, init)
// pass nil — they never execute a verb.
func New(set *discover.Set, providers map[string]model.ProviderConfig, env map[string]any) (*Registry, error) {
	r := &Registry{
		instances: map[string]*Instance{},
		builtins:  builtins.Specs(),
	}
	for name, cfg := range providers {
		inst, err := r.configure(set, name, cfg, env)
		if err != nil {
			return nil, err
		}
		r.instances[name] = inst
	}
	// Composed verbs can only be kind-inferred once every instance exists.
	for _, inst := range r.instances {
		if inst.Def != nil {
			if err := r.inferComposedKinds(inst); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}

func (r *Registry) configure(set *discover.Set, name string, cfg model.ProviderConfig, env map[string]any) (*Instance, error) {
	if cfg.Use != "" {
		def, err := set.FindLocalProvider(cfg.Use)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", name, err)
		}
		return &Instance{Name: name, Def: def, specs: map[string]sdk.VerbSpec{}}, nil
	}
	// Native: the type is the source when given (named instances of one
	// type: appA/appB), else the instance name itself.
	typeName := cfg.Source
	if typeName == "" {
		typeName = name
	}
	factory, ok := sdk.Factory(typeName)
	if !ok {
		return nil, fmt.Errorf("provider %q: unknown provider type %q (registered types: %s; composed providers need use:)", name, typeName, strings.Join(sdk.ProviderTypes(), ", "))
	}
	p := factory()
	cfgMap := make(map[string]any, len(cfg.Config)+1)
	for k, v := range cfg.Config {
		cfgMap[k] = v
	}
	// Providers resolving relative paths (exec scripts, compose files)
	// anchor on the project root, not the process cwd.
	if _, has := cfgMap["projectDir"]; !has && set.Project != nil {
		cfgMap["projectDir"] = set.Project.Dir
	}
	// The resolved project env reaches providers that shell out (docker, exec)
	// so ${VAR} interpolation in compose files and subprocess environments is
	// sourced from the declared env: block, not just the ambient process env.
	// projectEnv is a Shinari-owned reserved key, like projectDir.
	if len(env) > 0 {
		cfgMap["projectEnv"] = env
	}
	if err := p.Configure(cfgMap); err != nil {
		return nil, fmt.Errorf("provider %q: configure: %w", name, err)
	}
	specs := map[string]sdk.VerbSpec{}
	for _, vs := range p.Verbs() {
		specs[vs.Name] = vs
	}
	return &Instance{Name: name, Native: p, specs: specs}, nil
}

// Resolve binds a run: value. Unprefixed names are language builtins;
// everything else is <instance>.<verb>.
func (r *Registry) Resolve(run string) (Resolution, error) {
	if !strings.Contains(run, ".") {
		if spec, ok := r.builtins[run]; ok {
			return Resolution{Builtin: run, Spec: spec}, nil
		}
		return Resolution{}, fmt.Errorf("%q is not a language builtin (assert, sleep, wait_until, background, stop_background) — provider verbs are namespaced <provider>.<verb>", run)
	}
	parts := strings.SplitN(run, ".", 2)
	inst, ok := r.instances[parts[0]]
	if !ok {
		return Resolution{}, fmt.Errorf("verb %q: no provider instance named %q is configured", run, parts[0])
	}
	if inst.Def != nil {
		cv, ok := inst.Def.Verbs[parts[1]]
		if !ok {
			return Resolution{}, fmt.Errorf("verb %q: composed provider %q has no verb %q", run, parts[0], parts[1])
		}
		return Resolution{Instance: inst, Spec: inst.specs[parts[1]], Composed: &cv}, nil
	}
	spec, ok := inst.specs[parts[1]]
	if !ok {
		return Resolution{}, fmt.Errorf("verb %q: provider %q (type %s) has no verb %q", run, parts[0], inst.Native.Type(), parts[1])
	}
	return Resolution{Instance: inst, Spec: spec}, nil
}

// Close releases every native instance implementing sdk.Closer. The engine
// closes the registry after each scenario, so a multi-scenario run never leaks a
// connection pool per scenario. Errors are aggregated; non-Closer instances are
// skipped.
func (r *Registry) Close() error {
	var errs []error
	for name, inst := range r.instances {
		if c, ok := inst.Native.(sdk.Closer); ok {
			if err := c.Close(); err != nil {
				errs = append(errs, fmt.Errorf("provider %q: close: %w", name, err))
			}
		}
	}
	return errors.Join(errs...)
}

// Instances returns configured instance names (sorted, for messages).
func (r *Registry) Instances() []string {
	out := make([]string, 0, len(r.instances))
	for k := range r.instances {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Lifecycle returns the native instances exposing both up and down — the
// lifecycle capability (the runtime is a provider; exactly one required).
func (r *Registry) Lifecycle() []string {
	var out []string
	for name, inst := range r.instances {
		if inst.Native == nil {
			continue
		}
		if _, up := inst.specs["up"]; up {
			if _, down := inst.specs["down"]; down {
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// inferComposedKinds computes each macro verb's kind: a mutating
// leaf ⇒ action; else an assert leaf ⇒ assertion; else probe. It also
// enforces macro nesting ≤ 1 (composed may call composed only one level).
func (r *Registry) inferComposedKinds(inst *Instance) error {
	for name, cv := range inst.Def.Verbs {
		kind, sideEffects, effect, err := r.bodyKind(inst, name, cv, 0)
		if err != nil {
			return err
		}
		names, optional := cv.ParamNames()
		spec := sdk.VerbSpec{Name: name, Kind: kind, SideEffects: sideEffects, Effect: effect}
		for _, p := range names {
			spec.Args = append(spec.Args, sdk.ArgSpec{Name: p, Type: "any", Required: !optional[p]})
		}
		if len(names) > 0 {
			spec.Primary = names[0]
		}
		inst.specs[name] = spec
	}
	return nil
}

func (r *Registry) bodyKind(inst *Instance, verbName string, cv model.ComposedVerb, depth int) (sdk.Kind, bool, sdk.Effect, error) {
	steps := cv.Do
	if cv.Probe != nil {
		steps = []model.Step{*cv.Probe}
	}
	sawAssert := false
	sawMutation := false
	effect := sdk.EffectNone
	for _, st := range steps {
		// A step-level effect override (a fault injected via exec.run/http.post)
		// declares the fault where it lives and wins over the leaf's own effect.
		if st.Effect != "" {
			effect = strongerEffect(effect, sdk.Effect(st.Effect))
		}
		// The step-level kind override (for exec.run) also drives
		// inference: an exec.run marked probe/assertion does not mutate.
		if st.Kind != "" {
			switch sdk.Kind(st.Kind) {
			case sdk.KindAction:
				sawMutation = true
			case sdk.KindAssertion:
				sawAssert = true
			}
			continue
		}
		if !strings.Contains(st.Run, ".") {
			if st.Run == "assert" {
				sawAssert = true
			}
			continue
		}
		parts := strings.SplitN(st.Run, ".", 2)
		leafInst, ok := r.instances[parts[0]]
		if !ok {
			return "", false, "", fmt.Errorf("provider %q verb %q: calls %q but no instance %q is configured", inst.Name, verbName, st.Run, parts[0])
		}
		if leafInst.Def != nil {
			if depth >= 1 {
				return "", false, "", fmt.Errorf("provider %q verb %q: composed verbs may call another composed verb only one level deep (%q exceeds it)", inst.Name, verbName, st.Run)
			}
			nested, ok := leafInst.Def.Verbs[parts[1]]
			if !ok {
				return "", false, "", fmt.Errorf("provider %q verb %q: composed provider %q has no verb %q", inst.Name, verbName, parts[0], parts[1])
			}
			k, se, eff, err := r.bodyKind(leafInst, parts[1], nested, depth+1)
			if err != nil {
				return "", false, "", err
			}
			if se {
				sawMutation = true
			}
			if k == sdk.KindAssertion {
				sawAssert = true
			}
			if st.Effect == "" {
				effect = strongerEffect(effect, eff)
			}
			continue
		}
		spec, ok := leafInst.specs[parts[1]]
		if !ok {
			return "", false, "", fmt.Errorf("provider %q verb %q: provider %q has no verb %q", inst.Name, verbName, parts[0], parts[1])
		}
		if spec.SideEffects {
			sawMutation = true
		}
		if spec.Kind == sdk.KindAssertion {
			sawAssert = true
		}
		if st.Effect == "" {
			effect = strongerEffect(effect, spec.Effect)
		}
	}
	switch {
	case sawMutation:
		return sdk.KindAction, true, effect, nil
	case sawAssert:
		return sdk.KindAssertion, false, effect, nil
	default:
		return sdk.KindProbe, false, effect, nil
	}
}

// effectRank orders effect severity: outage outranks degradation outranks none.
var effectRank = map[sdk.Effect]int{sdk.EffectNone: 0, sdk.EffectDegradation: 1, sdk.EffectOutage: 2}

// strongerEffect returns the more severe of two effects: a macro that wraps a
// fault inherits it.
func strongerEffect(a, b sdk.Effect) sdk.Effect {
	if effectRank[b] > effectRank[a] {
		return b
	}
	return a
}

// BindArgs shapes an interpolated with: value into the verb's args map:
// scalar/list shorthand binds to Primary; a map is checked against the
// arg spec (unknown and missing-required are errors).
func BindArgs(spec sdk.VerbSpec, with any) (map[string]any, error) {
	args := map[string]any{}
	switch v := with.(type) {
	case nil:
		// no args
	case map[string]any:
		args = v
	default:
		if spec.Primary == "" {
			return nil, fmt.Errorf("verb %s takes named args (a map), got %T", spec.Name, with)
		}
		args[spec.Primary] = v
	}
	if len(spec.Args) > 0 {
		declared := map[string]bool{}
		for _, a := range spec.Args {
			declared[a.Name] = true
		}
		for k := range args {
			if !declared[k] {
				names := make([]string, 0, len(spec.Args))
				for _, a := range spec.Args {
					names = append(names, a.Name)
				}
				return nil, fmt.Errorf("verb %s: unknown arg %q (args: %s)", spec.Name, k, strings.Join(names, ", "))
			}
		}
		for _, a := range spec.Args {
			if a.Required {
				if _, ok := args[a.Name]; !ok {
					return nil, fmt.Errorf("verb %s: missing required arg %q", spec.Name, a.Name)
				}
			}
		}
	}
	return args, nil
}
