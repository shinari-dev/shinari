// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package netp

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestBlackholeUsesUnroutableAddress(t *testing.T) {
	dir := t.TempDir()
	p := provider(t, map[string]any{"confDir": dir})
	res, err := p.Run(context.Background(), "dns_blackhole", map[string]any{"host": "api.test"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(res.Value.(string))
	// 0.0.0.0 is not a blackhole: on Linux connect() to it reaches the local
	// host, so a local listener would answer the "outage".
	if string(data) != "address=/api.test/192.0.2.1\n" {
		t.Errorf("conf = %q, want the TEST-NET-1 blackhole", data)
	}
}

func TestBlackholeIPIsConfigurable(t *testing.T) {
	dir := t.TempDir()
	p := provider(t, map[string]any{"confDir": dir, "blackholeIp": "240.0.0.1"})
	res, err := p.Run(context.Background(), "dns_blackhole", map[string]any{"host": "api.test"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(res.Value.(string))
	if string(data) != "address=/api.test/240.0.0.1\n" {
		t.Errorf("conf = %q", data)
	}
}

func TestBlackholeIPRejectsNonIP(t *testing.T) {
	p := New().(*Provider)
	err := p.Configure(map[string]any{"confDir": t.TempDir(), "blackholeIp": "not-an-ip"})
	if err == nil {
		t.Fatal("want Configure error for a non-IP blackholeIp")
	}
}

func TestSanitizedHostsDoNotShareSnippetFiles(t *testing.T) {
	dir := t.TempDir()
	p := provider(t, map[string]any{"confDir": dir})
	// a_b.test and a*b.test sanitize to the same name: they must still own
	// distinct files, or clearing one would lift the other's fault.
	r1, err := p.Run(context.Background(), "nxdomain", map[string]any{"host": "a_b.test"})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := p.Run(context.Background(), "dns_blackhole", map[string]any{"host": "a*b.test"})
	if err != nil {
		t.Fatal(err)
	}
	if r1.Value.(string) == r2.Value.(string) {
		t.Fatalf("distinct hosts share snippet file %v", r1.Value)
	}
	if _, err := p.Run(context.Background(), "clear", map[string]any{"host": "a*b.test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r1.Value.(string)); err != nil {
		t.Error("clearing a*b.test lifted a_b.test's fault")
	}
	if _, err := os.Stat(r2.Value.(string)); !os.IsNotExist(err) {
		t.Error("clear did not remove a*b.test's own snippet")
	}
}

// dnsStub serves a minimal DNS-over-UDP responder: A records for known names,
// NXDOMAIN for everything else — just enough wire format for the pure-Go
// resolver, so resolve tests stay hermetic.
func dnsStub(t *testing.T, records map[string][]string) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 1024)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if resp := dnsAnswer(buf[:n], records); resp != nil {
				_, _ = pc.WriteTo(resp, addr)
			}
		}
	}()
	return pc.LocalAddr().String()
}

// dnsAnswer builds the response for one query: header + echoed question +
// one A answer per known address (compression pointer 0xC00C to the qname).
func dnsAnswer(q []byte, records map[string][]string) []byte {
	if len(q) < 12 {
		return nil
	}
	i := 12
	var labels []string
	for i < len(q) && q[i] != 0 {
		l := int(q[i])
		if i+1+l > len(q) {
			return nil
		}
		labels = append(labels, string(q[i+1:i+1+l]))
		i += 1 + l
	}
	if i+5 > len(q) {
		return nil
	}
	qtype := int(q[i+1])<<8 | int(q[i+2])
	qEnd := i + 5
	name := strings.ToLower(strings.Join(labels, "."))
	ips, known := records[name]

	rcode := byte(0)
	if !known {
		rcode = 3 // NXDOMAIN
	}
	var answers []byte
	count := 0
	if qtype == 1 { // A
		for _, s := range ips {
			ip := net.ParseIP(s).To4()
			if ip == nil {
				continue
			}
			answers = append(answers, 0xC0, 0x0C, 0, 1, 0, 1, 0, 0, 0, 30, 0, 4)
			answers = append(answers, ip...)
			count++
		}
	}
	resp := make([]byte, 0, qEnd+len(answers))
	resp = append(resp, q[0], q[1], 0x85, 0x80|rcode, 0, 1, byte(count>>8), byte(count), 0, 0, 0, 0)
	resp = append(resp, q[12:qEnd]...)
	resp = append(resp, answers...)
	return resp
}

func TestResolveObservesOverride(t *testing.T) {
	server := dnsStub(t, map[string][]string{"db.internal": {"10.0.0.9", "10.0.0.10"}})
	p := provider(t, map[string]any{"confDir": t.TempDir()})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := p.Run(ctx, "resolve", map[string]any{"host": "db.internal", "server": server})
	if err != nil {
		t.Fatal(err)
	}
	got := map[any]bool{}
	for _, v := range res.Value.([]any) {
		got[v] = true
	}
	if len(got) != 2 || !got["10.0.0.9"] || !got["10.0.0.10"] {
		t.Fatalf("value = %v, want both A records", res.Value)
	}
	if res.Meta["count"] != 2 || res.Meta["nxdomain"] != false {
		t.Errorf("meta = %v", res.Meta)
	}
}

func TestResolveNXDomainIsObservationNotError(t *testing.T) {
	// The nxdomain fault being live is what the step came to see: an empty
	// answer set, not a probe failure.
	server := dnsStub(t, map[string][]string{})
	p := provider(t, map[string]any{"confDir": t.TempDir()})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := p.Run(ctx, "resolve", map[string]any{"host": "gone.test", "server": server})
	if err != nil {
		t.Fatalf("NXDOMAIN must be an observation, got %v", err)
	}
	if list := res.Value.([]any); len(list) != 0 {
		t.Errorf("value = %v, want an empty list", list)
	}
	if res.Meta["nxdomain"] != true || res.Output != "NXDOMAIN" {
		t.Errorf("meta = %v output = %q", res.Meta, res.Output)
	}
}

func TestResolveTransportFailureIsError(t *testing.T) {
	// A DNS server that is not there at all is a transport failure, not a
	// resolution observation.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := pc.LocalAddr().String()
	_ = pc.Close()

	p := provider(t, map[string]any{"confDir": t.TempDir()})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := p.Run(ctx, "resolve", map[string]any{"host": "db.internal", "server": dead}); err == nil {
		t.Fatal("an unreachable DNS server must surface as an error")
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
