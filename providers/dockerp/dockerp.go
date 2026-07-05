// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package dockerp is the docker built-in provider: lifecycle + process
// faults via the docker compose CLI. The CLI is the stable compose
// contract; there is no first-party compose Go SDK.
package dockerp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

type Provider struct {
	composeFiles []string
	project      string
	bin          string // overridable for tests
	binArgs      []string
	env          []string // resolved project env as KEY=VALUE, forwarded to compose/docker subprocesses
}

func init() { sdk.Register("docker", New) }

func New() sdk.Provider { return &Provider{bin: "docker", binArgs: []string{"compose"}} }

func (p *Provider) Type() string { return "docker" }

func (p *Provider) Configure(cfg map[string]any) error {
	projectDir, _ := cfg["projectDir"].(string)
	if files, ok := cfg["composeFiles"].([]any); ok {
		for _, f := range files {
			path := fmt.Sprintf("%v", f)
			if projectDir != "" && !filepath.IsAbs(path) {
				path = filepath.Join(projectDir, path)
			}
			p.composeFiles = append(p.composeFiles, path)
		}
	}
	if proj, ok := cfg["project"].(string); ok {
		p.project = proj
	}
	// test seam: bin: ["/path/to/stub"] replaces the docker compose binary
	if bin, ok := cfg["bin"].(string); ok && bin != "" {
		p.bin = bin
		p.binArgs = nil
	}
	// The resolved project env feeds compose's own ${VAR} interpolation: a
	// compose file that references ${APP_IMAGE} sees the value declared in
	// env: (sourced from .env/--env-file/OS) without the caller sourcing it into
	// the shell first. Sorted so the assembled environment is deterministic.
	p.env = envKV(cfg["projectEnv"])
	return nil
}

// envKV renders a projectEnv map into sorted KEY=VALUE strings. Sorting is only
// for determinism; env keys are unique, so order never changes the result.
func envKV(v any) []string {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	kv := make([]string, 0, len(m))
	for k, val := range m {
		kv = append(kv, fmt.Sprintf("%s=%v", k, val))
	}
	sort.Strings(kv)
	return kv
}

// processEnv is the environment for a shelled-out command: the inherited process
// env with the resolved project env appended, so project-declared values win
// over ambient ones (os/exec uses the last value for a duplicate key). Returns
// nil when there is no project env, leaving the child to inherit os.Environ().
func (p *Provider) processEnv() []string {
	if len(p.env) == 0 {
		return nil
	}
	return append(os.Environ(), p.env...)
}

func (p *Provider) Verbs() []sdk.VerbSpec {
	upArgs := []sdk.ArgSpec{{Name: "services", Type: "list"}, {Name: "wait", Type: "bool"}, {Name: "profiles", Type: "list"}}
	service := []sdk.ArgSpec{{Name: "service", Type: "string", Required: true}}
	networkArgs := []sdk.ArgSpec{{Name: "service", Type: "string", Required: true}, {Name: "network", Type: "string"}}
	return []sdk.VerbSpec{
		{Name: "up", Kind: sdk.KindAction, SideEffects: true, Primary: "services", Args: upArgs},
		{Name: "down", Kind: sdk.KindAction, SideEffects: true},
		{Name: "kill", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "service", Args: service},
		{Name: "stop", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "service", Args: service},
		{Name: "start", Kind: sdk.KindAction, SideEffects: true, Primary: "service", Args: service},
		// restart bounces a service (stop + start) in one step: the graceful
		// rolling-restart fault. An outage — work in flight when the SIGTERM
		// lands is dropped — but one that heals itself, so the interesting
		// assertions are about what peers observed during the bounce.
		{Name: "restart", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "service", Args: service},
		{Name: "pause", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "service", Args: service},
		{Name: "unpause", Kind: sdk.KindAction, SideEffects: true, Primary: "service", Args: service},
		{Name: "logs", Kind: sdk.KindProbe, Primary: "service", Args: []sdk.ArgSpec{
			{Name: "service", Type: "string", Required: true},
			{Name: "tail", Type: "string"},
			{Name: "since", Type: "string"},
		}},
		{Name: "ps", Kind: sdk.KindProbe, Primary: "service", Args: []sdk.ArgSpec{
			{Name: "service", Type: "string"},
		}},
		// exec runs a command inside a running container and returns its stdout,
		// so a scenario can read internal runtime state (thread/fd counts,
		// memory, an in-container metric) and baseline-then-compare it with the
		// standard assert operators. A probe: it observes, it does not inject a
		// fault — keep the command read-only.
		{Name: "exec", Kind: sdk.KindProbe, Primary: "command", Args: []sdk.ArgSpec{
			{Name: "service", Type: "string", Required: true},
			{Name: "command", Type: "string", Required: true},
		}},
		// disconnect/connect partition a single container at the network layer:
		// the process keeps running and co-located peers are untouched, so the
		// scenario observes last-known-state behavior and reconnection on
		// restore — a distinct failure mode from kill/stop/pause. They target
		// one network (the compose default unless network: is given); a
		// multi-network container is isolated by disconnecting each.
		{Name: "disconnect", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectOutage, Primary: "service", Args: networkArgs},
		{Name: "connect", Kind: sdk.KindAction, SideEffects: true, Primary: "service", Args: networkArgs},
		// throttle/unthrottle cap and restore a container's CPU via
		// `docker update --cpus`: resource starvation as a degradation — the
		// process keeps running and keeps its connections, it just gets slow.
		// CPU only: a memory ceiling cannot be reset to unlimited through
		// `docker update`, so it would be a fault with no restore; inject
		// memory pressure by restarting the service with compose-level limits.
		{Name: "throttle", Kind: sdk.KindAction, SideEffects: true, Effect: sdk.EffectDegradation, Primary: "service",
			Args: []sdk.ArgSpec{
				{Name: "service", Type: "string", Required: true},
				{Name: "cpus", Type: "number", Required: true},
			}},
		{Name: "unthrottle", Kind: sdk.KindAction, SideEffects: true, Primary: "service", Args: service},
	}
}

// parsePS decodes `compose ps --format json`, which emits either a JSON array
// or one JSON object per line depending on the compose version.
func parsePS(out string) ([]any, error) {
	s := strings.TrimSpace(out)
	if s == "" {
		return []any{}, nil
	}
	if strings.HasPrefix(s, "[") {
		var arr []any
		if err := json.Unmarshal([]byte(s), &arr); err != nil {
			return nil, err
		}
		return arr, nil
	}
	var arr []any
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return nil, err
		}
		arr = append(arr, obj)
	}
	return arr, nil
}

func (p *Provider) compose(ctx context.Context, args ...string) (string, error) {
	full := append([]string{}, p.binArgs...)
	for _, f := range p.composeFiles {
		full = append(full, "-f", f)
	}
	if p.project != "" {
		full = append(full, "-p", p.project)
	}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, p.bin, full...)
	cmd.Env = p.processEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker compose %s: %w — %s",
			strings.Join(args, " "), err, conv.Truncate(strings.TrimSpace(string(out)), 300))
	}
	return string(out), nil
}

// docker runs a raw docker command (not a compose subcommand): no `compose`
// prefix, no -f/-p. `docker network disconnect|connect` has no compose
// equivalent, so a partition reaches the daemon directly.
func (p *Provider) docker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, p.bin, args...)
	cmd.Env = p.processEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker %s: %w — %s",
			strings.Join(args, " "), err, conv.Truncate(strings.TrimSpace(string(out)), 300))
	}
	return string(out), nil
}

// containerIDs resolves a compose service to its running container IDs, robust
// to generated names and scaled replicas (`compose ps -q` prints one per line).
func (p *Provider) containerIDs(ctx context.Context, service string) ([]string, error) {
	out, err := p.compose(ctx, "ps", "-q", service)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			ids = append(ids, line)
		}
	}
	return ids, nil
}

// networkToggle disconnects a service's containers from a docker network (the
// partition fault) or reconnects them (its restore). With no network it
// defaults to compose's <project>_default. It returns the last command's output
// and stops at the first failure.
func (p *Provider) networkToggle(ctx context.Context, verb, service string, args map[string]any) (string, error) {
	network, _ := args["network"].(string)
	if network == "" {
		// compose names its default network <project>_default; without a known
		// project name there is nothing to default to.
		if p.project == "" {
			return "", fmt.Errorf("docker %s: a network is required (no compose project set to derive <project>_default)", verb)
		}
		network = p.project + "_default"
	}
	ids, err := p.containerIDs(ctx, service)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("docker %s: no running container for service %q", verb, service)
	}
	var out string
	for _, id := range ids {
		if verb == "disconnect" {
			// -f forces the disconnect even if the daemon thinks the endpoint is
			// busy, so the partition takes effect reliably.
			out, err = p.docker(ctx, "network", "disconnect", "-f", network, id)
		} else {
			// --alias restores the compose service-name DNS alias, which a manual
			// connect would otherwise drop, so peers resolve it again.
			out, err = p.docker(ctx, "network", "connect", "--alias", service, network, id)
		}
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// updateCPUs applies (throttle) or removes (unthrottle) a CPU ceiling on a
// service's running containers. `docker update --cpus 0` means "no limit", so
// unthrottle is throttle's fixed restore.
func (p *Provider) updateCPUs(ctx context.Context, verb, service string, args map[string]any) (string, error) {
	cpus := "0"
	if verb == "throttle" {
		f, ok := conv.ToFloat(args["cpus"])
		if !ok || f <= 0 {
			return "", fmt.Errorf("docker throttle: needs cpus: (a positive CPU ceiling, e.g. 0.2)")
		}
		cpus = strconv.FormatFloat(f, 'f', -1, 64)
	}
	ids, err := p.containerIDs(ctx, service)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("docker %s: no running container for service %q", verb, service)
	}
	return p.docker(ctx, append([]string{"update", "--cpus", cpus}, ids...)...)
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	service, _ := args["service"].(string)
	var out string
	var err error
	switch verb {
	case "up":
		// --profile selects a compose profile (a service variant), so one
		// compose file can carry worker/worker-rr/worker-pf and a scenario picks
		// one. It is a top-level flag, so it precedes the up subcommand.
		var cmdArgs []string
		if profiles, ok := args["profiles"].([]any); ok {
			for _, p := range profiles {
				cmdArgs = append(cmdArgs, "--profile", fmt.Sprintf("%v", p))
			}
		}
		cmdArgs = append(cmdArgs, "up", "-d")
		// --wait blocks until services are healthy; opt out with wait: false to
		// start a service that is meant to crash or hang (--wait would abort the
		// step before any assertion could observe it).
		if w, ok := args["wait"].(bool); !ok || w {
			cmdArgs = append(cmdArgs, "--wait")
		}
		if list, ok := args["services"].([]any); ok {
			for _, s := range list {
				cmdArgs = append(cmdArgs, fmt.Sprintf("%v", s))
			}
		}
		out, err = p.compose(ctx, cmdArgs...)
	case "down":
		out, err = p.compose(ctx, "down", "-v", "--remove-orphans")
	case "kill":
		out, err = p.compose(ctx, "kill", "-s", "SIGKILL", service)
	case "stop":
		out, err = p.compose(ctx, "stop", service)
	case "start":
		out, err = p.compose(ctx, "start", service)
	case "restart":
		out, err = p.compose(ctx, "restart", service)
	case "throttle", "unthrottle":
		out, err = p.updateCPUs(ctx, verb, service, args)
	case "pause":
		out, err = p.compose(ctx, "pause", service)
	case "unpause":
		out, err = p.compose(ctx, "unpause", service)
	case "exec":
		// -T disables TTY allocation (no interactive terminal); sh -c runs the
		// command string so pipes/globs (e.g. `ls /proc/1/task | wc -l`) work.
		command, _ := args["command"].(string)
		out, err = p.compose(ctx, "exec", "-T", service, "sh", "-c", command)
	case "disconnect", "connect":
		out, err = p.networkToggle(ctx, verb, service, args)
	case "logs":
		cmdArgs := []string{"logs", "--no-color"}
		if t, ok := args["tail"]; ok && t != nil && fmt.Sprintf("%v", t) != "" {
			cmdArgs = append(cmdArgs, "--tail", fmt.Sprintf("%v", t))
		}
		if s, ok := args["since"].(string); ok && s != "" {
			cmdArgs = append(cmdArgs, "--since", s)
		}
		cmdArgs = append(cmdArgs, service)
		out, err = p.compose(ctx, cmdArgs...)
	case "ps":
		// --all so exited/dead containers still report (a crashed worker is
		// exactly what a fail-fast check wants to inspect).
		cmdArgs := []string{"ps", "--all", "--format", "json"}
		if service != "" {
			cmdArgs = append(cmdArgs, service)
		}
		out, err = p.compose(ctx, cmdArgs...)
		if err != nil {
			return sdk.VerbResult{Output: out}, err
		}
		list, perr := parsePS(out)
		if perr != nil {
			return sdk.VerbResult{Output: out}, fmt.Errorf("docker ps: parsing --format json: %w — %s", perr, conv.Truncate(strings.TrimSpace(out), 200))
		}
		meta := map[string]any{"count": len(list)}
		// A single named service binds its object directly, so read:/capture:
		// reach .State / .ExitCode / .Health without indexing a one-element list.
		var value any = list
		if service != "" && len(list) == 1 {
			value = list[0]
		}
		return sdk.VerbResult{Value: value, Output: out, Meta: meta}, nil
	default:
		return sdk.VerbResult{}, fmt.Errorf("docker has no verb %q", verb)
	}
	if err != nil {
		return sdk.VerbResult{Output: out}, err
	}
	return sdk.VerbResult{Value: strings.TrimSpace(out), Output: out}, nil
}
