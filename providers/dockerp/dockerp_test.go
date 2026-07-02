// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package dockerp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stub records its argv and prints canned output.
func stubBin(t *testing.T) (bin string, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "fake-docker")
	argsFile = filepath.Join(dir, "args.txt")
	script := "#!/bin/sh\necho \"$@\" >> " + argsFile + "\necho stub-ok\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argsFile
}

func provider(t *testing.T) (*Provider, string) {
	t.Helper()
	bin, argsFile := stubBin(t)
	p := New().(*Provider)
	if err := p.Configure(map[string]any{
		"composeFiles": []any{"assets/stack.yml"},
		"project":      "chaos-run",
		"bin":          bin,
	}); err != nil {
		t.Fatal(err)
	}
	return p, argsFile
}

func recorded(t *testing.T, argsFile string) string {
	t.Helper()
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func TestUpBuildsComposeCommand(t *testing.T) {
	p, argsFile := provider(t)
	res, err := p.Run(context.Background(), "up", map[string]any{"services": []any{"postgres", "app"}})
	if err != nil {
		t.Fatal(err)
	}
	got := recorded(t, argsFile)
	want := "-f assets/stack.yml -p chaos-run up -d --wait postgres app"
	if got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
	if res.Value != "stub-ok" {
		t.Errorf("value = %v", res.Value)
	}
}

func TestKillUsesSIGKILL(t *testing.T) {
	p, argsFile := provider(t)
	if _, err := p.Run(context.Background(), "kill", map[string]any{"service": "worker-a"}); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, argsFile); !strings.Contains(got, "kill -s SIGKILL worker-a") {
		t.Errorf("argv = %q", got)
	}
}

func TestDownRemovesVolumesAndOrphans(t *testing.T) {
	p, argsFile := provider(t)
	if _, err := p.Run(context.Background(), "down", nil); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, argsFile); !strings.Contains(got, "down -v --remove-orphans") {
		t.Errorf("argv = %q", got)
	}
}

func TestLogsCursorArgs(t *testing.T) {
	p, argsFile := provider(t)
	_, err := p.Run(context.Background(), "logs", map[string]any{
		"service": "api", "tail": 50, "since": "30s",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := recorded(t, argsFile)
	for _, want := range []string{"logs", "--no-color", "--tail 50", "--since 30s", "api"} {
		if !strings.Contains(got, want) {
			t.Errorf("argv %q missing %q", got, want)
		}
	}
}

func TestFailureSurfacesOutput(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fail-docker")
	_ = os.WriteFile(bin, []byte("#!/bin/sh\necho 'no such service: ghost' >&2\nexit 1\n"), 0o755)
	p := New().(*Provider)
	_ = p.Configure(map[string]any{"bin": bin})
	_, err := p.Run(context.Background(), "stop", map[string]any{"service": "ghost"})
	if err == nil || !strings.Contains(err.Error(), "no such service") {
		t.Fatalf("want compose stderr in error, got %v", err)
	}
}

func TestUpWaitFalseOmitsWait(t *testing.T) {
	p, argsFile := provider(t)
	if _, err := p.Run(context.Background(), "up", map[string]any{
		"services": []any{"worker"}, "wait": false,
	}); err != nil {
		t.Fatal(err)
	}
	got := recorded(t, argsFile)
	if strings.Contains(got, "--wait") {
		t.Errorf("argv = %q, should not contain --wait when wait:false", got)
	}
	if !strings.Contains(got, "up -d worker") {
		t.Errorf("argv = %q, want 'up -d worker'", got)
	}
}

// jsonStubProvider returns a docker provider whose stub emits canned NDJSON,
// the shape `compose ps --format json` produces.
func jsonStubProvider(t *testing.T, line string) *Provider {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-docker")
	script := "#!/bin/sh\nprintf '%s\\n' '" + line + "'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	p := New().(*Provider)
	if err := p.Configure(map[string]any{"bin": bin}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPsNamedServiceBindsExitState(t *testing.T) {
	p := jsonStubProvider(t, `{"Service":"worker","State":"exited","ExitCode":0}`)
	res, err := p.Run(context.Background(), "ps", map[string]any{"service": "worker"})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := res.Value.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v, want a single object for a named service", res.Value)
	}
	if m["State"] != "exited" || m["ExitCode"] != float64(0) {
		t.Errorf("ps state = %v / exit = %v (%T)", m["State"], m["ExitCode"], m["ExitCode"])
	}
}

func TestPsNoServiceReturnsList(t *testing.T) {
	p := jsonStubProvider(t, `{"Service":"a","State":"running"}`)
	res, err := p.Run(context.Background(), "ps", nil)
	if err != nil {
		t.Fatal(err)
	}
	arr, ok := res.Value.([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("value = %#v, want a one-element list", res.Value)
	}
}

func TestExecRunsCommandInContainer(t *testing.T) {
	p, argsFile := provider(t)
	res, err := p.Run(context.Background(), "exec", map[string]any{
		"service": "worker", "command": "ls /proc/1/task | wc -l",
	})
	if err != nil {
		t.Fatal(err)
	}
	// -T (no TTY) and sh -c so the pipeline runs inside the container.
	if got := recorded(t, argsFile); !strings.Contains(got, "exec -T worker sh -c ls /proc/1/task | wc -l") {
		t.Errorf("argv = %q", got)
	}
	if res.Value != "stub-ok" {
		t.Errorf("value = %v, want trimmed stdout", res.Value)
	}
}

func TestDisconnectPartitionsContainer(t *testing.T) {
	p, argsFile := provider(t)
	if _, err := p.Run(context.Background(), "disconnect", map[string]any{"service": "worker"}); err != nil {
		t.Fatal(err)
	}
	got := recorded(t, argsFile)
	// resolves the container id via compose ps -q...
	if !strings.Contains(got, "ps -q worker") {
		t.Errorf("argv %q missing container resolve", got)
	}
	// ...then force-severs it from the compose default network (the stub
	// returns "stub-ok" as the resolved container id).
	if !strings.Contains(got, "network disconnect -f chaos-run_default stub-ok") {
		t.Errorf("argv %q missing forced network disconnect", got)
	}
}

func TestConnectRestoresServiceAlias(t *testing.T) {
	p, argsFile := provider(t)
	if _, err := p.Run(context.Background(), "connect", map[string]any{
		"service": "worker", "network": "custom_net",
	}); err != nil {
		t.Fatal(err)
	}
	got := recorded(t, argsFile)
	// connect re-attaches and restores the service-name DNS alias.
	if !strings.Contains(got, "network connect --alias worker custom_net stub-ok") {
		t.Errorf("argv %q missing aliased network connect", got)
	}
}

func TestDisconnectNeedsNetworkWithoutProject(t *testing.T) {
	bin, _ := stubBin(t)
	p := New().(*Provider)
	if err := p.Configure(map[string]any{"bin": bin}); err != nil { // no project
		t.Fatal(err)
	}
	_, err := p.Run(context.Background(), "disconnect", map[string]any{"service": "worker"})
	if err == nil || !strings.Contains(err.Error(), "network is required") {
		t.Fatalf("want a network-required error without a compose project, got %v", err)
	}
}

func TestRestartBouncesService(t *testing.T) {
	p, argsFile := provider(t)
	if _, err := p.Run(context.Background(), "restart", map[string]any{"service": "api"}); err != nil {
		t.Fatal(err)
	}
	if got := recorded(t, argsFile); !strings.Contains(got, "restart api") {
		t.Errorf("argv = %q", got)
	}
}

func TestThrottleCapsCPU(t *testing.T) {
	p, argsFile := provider(t)
	if _, err := p.Run(context.Background(), "throttle", map[string]any{
		"service": "worker", "cpus": 0.2,
	}); err != nil {
		t.Fatal(err)
	}
	got := recorded(t, argsFile)
	// resolves the container id via compose ps -q...
	if !strings.Contains(got, "ps -q worker") {
		t.Errorf("argv %q missing container resolve", got)
	}
	// ...then caps it (the stub returns "stub-ok" as the resolved id).
	if !strings.Contains(got, "update --cpus 0.2 stub-ok") {
		t.Errorf("argv %q missing cpu cap", got)
	}
}

func TestUnthrottleRemovesCap(t *testing.T) {
	p, argsFile := provider(t)
	if _, err := p.Run(context.Background(), "unthrottle", map[string]any{"service": "worker"}); err != nil {
		t.Fatal(err)
	}
	// --cpus 0 means "no limit": the fixed restore.
	if got := recorded(t, argsFile); !strings.Contains(got, "update --cpus 0 stub-ok") {
		t.Errorf("argv = %q", got)
	}
}

func TestThrottleNeedsPositiveCPUs(t *testing.T) {
	p, _ := provider(t)
	for _, args := range []map[string]any{
		{"service": "worker"},
		{"service": "worker", "cpus": 0},
		{"service": "worker", "cpus": -1},
	} {
		if _, err := p.Run(context.Background(), "throttle", args); err == nil {
			t.Errorf("args %v: want error for missing/non-positive cpus", args)
		}
	}
}

func TestUpProfilesPrecedeSubcommand(t *testing.T) {
	p, argsFile := provider(t)
	if _, err := p.Run(context.Background(), "up", map[string]any{
		"profiles": []any{"rr"}, "services": []any{"worker"},
	}); err != nil {
		t.Fatal(err)
	}
	got := recorded(t, argsFile)
	// --profile is a top-level flag and must come before `up`.
	want := "-f assets/stack.yml -p chaos-run --profile rr up -d --wait worker"
	if got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
}
