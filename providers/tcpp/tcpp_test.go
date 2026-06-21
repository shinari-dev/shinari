// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package tcpp

import (
	"context"
	"net"
	"testing"
)

func TestConnectReachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	p := New()
	if err := p.Configure(map[string]any{"addr": ln.Addr().String()}); err != nil {
		t.Fatal(err)
	}
	res, err := p.Run(context.Background(), "connect", map[string]any{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if res.Value != true {
		t.Errorf("value = %v, want true", res.Value)
	}
	if _, ok := res.Meta["connectMs"]; !ok {
		t.Errorf("missing connectMs meta: %v", res.Meta)
	}
	if res.Meta["addr"] != ln.Addr().String() {
		t.Errorf("addr meta = %v", res.Meta["addr"])
	}
}

func TestConnectRefused(t *testing.T) {
	// Bind then close to obtain an address nothing is listening on.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	p := New()
	// Pass the dead address per-step (scalar shorthand binds to addr) with a
	// short timeout so the test never hangs.
	res, err := p.Run(context.Background(), "connect", map[string]any{"addr": addr, "timeout": 2})
	if err == nil {
		t.Fatalf("expected an error connecting to a closed port, got %+v", res)
	}
}

func TestConnectNeedsAddr(t *testing.T) {
	p := New()
	if _, err := p.Run(context.Background(), "connect", map[string]any{}); err == nil {
		t.Fatal("expected an error when no addr is configured or passed")
	}
}

func TestAddrFromHostPort(t *testing.T) {
	if got := addrOf(map[string]any{"host": "localhost", "port": 6379.0}); got != "localhost:6379" {
		t.Errorf("addrOf host/port = %q", got)
	}
	if got := addrOf(map[string]any{"addr": "db:5432"}); got != "db:5432" {
		t.Errorf("addrOf addr = %q", got)
	}
}
