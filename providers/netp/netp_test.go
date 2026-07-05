// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package netp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func provider(t *testing.T, cfg map[string]any) *Provider {
	t.Helper()
	p := New().(*Provider)
	if err := p.Configure(cfg); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSetDNSWritesConf(t *testing.T) {
	dir := t.TempDir()
	p := provider(t, map[string]any{"confDir": dir})
	res, err := p.Run(context.Background(), "set_dns", map[string]any{"host": "db.internal", "ip": "10.0.0.9"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(res.Value.(string))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "address=/db.internal/10.0.0.9\n" {
		t.Errorf("conf = %q", data)
	}
}

func TestSetDNSMultipleEndpoints(t *testing.T) {
	dir := t.TempDir()
	p := provider(t, map[string]any{"confDir": dir})
	res, err := p.Run(context.Background(), "set_dns", map[string]any{
		"host": "backends.example.test",
		"ips":  []any{"10.0.0.1", "10.0.0.2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(res.Value.(string))
	if err != nil {
		t.Fatal(err)
	}
	want := "address=/backends.example.test/10.0.0.1\naddress=/backends.example.test/10.0.0.2\n"
	if string(data) != want {
		t.Errorf("conf = %q, want %q", data, want)
	}
}

func TestSetDNSUnionsAndDedups(t *testing.T) {
	dir := t.TempDir()
	p := provider(t, map[string]any{"confDir": dir})
	// ip duplicates the first ips entry; order is ips-first, then ip if new.
	res, err := p.Run(context.Background(), "set_dns", map[string]any{
		"host": "ctrl.test",
		"ip":   "10.0.0.1",
		"ips":  []any{"10.0.0.1", "10.0.0.2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(res.Value.(string))
	want := "address=/ctrl.test/10.0.0.1\naddress=/ctrl.test/10.0.0.2\n"
	if string(data) != want {
		t.Errorf("conf = %q, want %q", data, want)
	}
}

func TestSetDNSEmptyIsError(t *testing.T) {
	dir := t.TempDir()
	p := provider(t, map[string]any{"confDir": dir})
	for _, args := range []map[string]any{
		{"host": "ctrl.test"},
		{"host": "ctrl.test", "ips": []any{}},
	} {
		_, err := p.Run(context.Background(), "set_dns", args)
		if err == nil || !strings.Contains(err.Error(), "nxdomain") {
			t.Errorf("args %v: want error pointing at nxdomain, got %v", args, err)
		}
	}
}

func TestNXDomain(t *testing.T) {
	dir := t.TempDir()
	p := provider(t, map[string]any{"confDir": dir})
	res, _ := p.Run(context.Background(), "nxdomain", map[string]any{"host": "api.partner.com"})
	data, _ := os.ReadFile(res.Value.(string))
	if string(data) != "address=/api.partner.com/\n" {
		t.Errorf("conf = %q", data)
	}
}

func TestReloadCmdRunsAndFailureSurfaces(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "reloaded")
	p := provider(t, map[string]any{"confDir": dir, "reloadCmd": "touch " + marker})
	if _, err := p.Run(context.Background(), "dns_blackhole", map[string]any{"host": "x.io"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Error("reloadCmd did not run")
	}

	p2 := provider(t, map[string]any{"confDir": dir, "reloadCmd": "echo dnsmasq gone >&2; exit 1"})
	_, err := p2.Run(context.Background(), "nxdomain", map[string]any{"host": "x.io"})
	if err == nil || !strings.Contains(err.Error(), "dnsmasq gone") {
		t.Fatalf("want reload failure surfaced, got %v", err)
	}
}

func TestClearLiftsOneHost(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "reloaded")
	p := provider(t, map[string]any{"confDir": dir, "reloadCmd": "touch " + marker})
	res, err := p.Run(context.Background(), "nxdomain", map[string]any{"host": "db.internal"})
	if err != nil {
		t.Fatal(err)
	}
	file := res.Value.(string)
	_ = os.Remove(marker)

	res, err = p.Run(context.Background(), "clear", map[string]any{"host": "db.internal"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("snippet %s still exists after clear", file)
	}
	if got := res.Value.([]any); len(got) != 1 || got[0] != file {
		t.Errorf("value = %v, want [%s]", got, file)
	}
	if res.Meta["removed"] != 1 {
		t.Errorf("meta.removed = %v, want 1", res.Meta["removed"])
	}
	if _, err := os.Stat(marker); err != nil {
		t.Error("clear did not reload")
	}
}

func TestClearIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	p := provider(t, map[string]any{"confDir": dir})
	// Restoring a fault that was never injected (teardown after an early
	// failure) must be a no-op, not an error.
	res, err := p.Run(context.Background(), "clear", map[string]any{"host": "never.faulted"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta["removed"] != 0 {
		t.Errorf("meta.removed = %v, want 0", res.Meta["removed"])
	}
}

func TestResetLiftsAllShinariSnippets(t *testing.T) {
	dir := t.TempDir()
	p := provider(t, map[string]any{"confDir": dir})
	for _, host := range []string{"a.test", "b.test"} {
		if _, err := p.Run(context.Background(), "dns_blackhole", map[string]any{"host": host}); err != nil {
			t.Fatal(err)
		}
	}
	// A foreign conf file in the same dir is not shinari's to remove.
	foreign := filepath.Join(dir, "upstream.conf")
	if err := os.WriteFile(foreign, []byte("server=1.1.1.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := p.Run(context.Background(), "reset", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta["removed"] != 2 {
		t.Errorf("meta.removed = %v, want 2", res.Meta["removed"])
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Error("reset removed a conf file it does not own")
	}
	left, _ := filepath.Glob(filepath.Join(dir, "shinari-*.conf"))
	if len(left) != 0 {
		t.Errorf("shinari snippets left after reset: %v", left)
	}
}

func TestMissingConfDirIsConfigureError(t *testing.T) {
	p := New().(*Provider)
	if err := p.Configure(map[string]any{}); err == nil {
		t.Fatal("want error for missing confDir")
	}
}

func TestHostWithInjectionCharactersIsRejected(t *testing.T) {
	p := provider(t, map[string]any{"confDir": t.TempDir()})
	for _, host := range []string{"a.test\nserver=6.6.6.6", "a/b.test", "a b"} {
		if _, err := p.Run(context.Background(), "nxdomain", map[string]any{"host": host}); err == nil {
			t.Errorf("host %q must be rejected before it reaches the snippet", host)
		}
	}
}

func TestSetDNSRejectsNonIP(t *testing.T) {
	p := provider(t, map[string]any{"confDir": t.TempDir()})
	if _, err := p.Run(context.Background(), "set_dns", map[string]any{"host": "a.test", "ip": "6.6.6.6\naddress=/b/1.1.1.1"}); err == nil {
		t.Error("a non-IP address value must be rejected")
	}
}
