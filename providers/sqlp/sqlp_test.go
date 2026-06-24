// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package sqlp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

func newTestProvider(t *testing.T) sdk.Provider {
	t.Helper()
	p := New()
	dsn := filepath.Join(t.TempDir(), "test.db")
	if err := p.Configure(map[string]any{"driver": "sqlite", "dsn": dsn}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	return p
}

func TestConfigureRejectsUnknownDriver(t *testing.T) {
	err := New().Configure(map[string]any{"driver": "oracle", "dsn": "x"})
	if err == nil || !strings.Contains(err.Error(), "unknown driver") {
		t.Fatalf("want unknown driver error, got %v", err)
	}
}

func TestConfigureRequiresDSN(t *testing.T) {
	err := New().Configure(map[string]any{"driver": "sqlite"})
	if err == nil || !strings.Contains(err.Error(), "dsn") {
		t.Fatalf("want dsn error, got %v", err)
	}
}

// TestConfigureAcceptsMysql proves the mysql driver is registered and opens
// lazily (sql.Open does not connect), matching the postgres path.
func TestConfigureAcceptsMysql(t *testing.T) {
	err := New().Configure(map[string]any{"driver": "mysql", "dsn": "user:pass@tcp(127.0.0.1:3306)/app"})
	if err != nil {
		t.Fatalf("mysql configure: %v", err)
	}
}

func TestExecThenQuery(t *testing.T) {
	p := newTestProvider(t)
	ctx := context.Background()
	mustExec(t, p, "CREATE TABLE runs(job_id TEXT, n INT)")
	mustExec(t, p, "INSERT INTO runs VALUES('j1', 1)")

	res, err := p.Run(ctx, "query", map[string]any{
		"sql":  "SELECT n FROM runs WHERE job_id=?",
		"args": []any{"j1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := res.Value.([]map[string]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("value=%v (%T)", res.Value, res.Value)
	}
	if got, _ := conv.ToFloat(rows[0]["n"]); got != 1 {
		t.Fatalf("n=%v", rows[0]["n"])
	}
}

func mustExec(t *testing.T, p sdk.Provider, q string) {
	t.Helper()
	if _, err := p.Run(context.Background(), "exec", map[string]any{"sql": q}); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func TestExecReportsRowsAffected(t *testing.T) {
	p := newTestProvider(t)
	mustExec(t, p, "CREATE TABLE t(x INT)")
	mustExec(t, p, "INSERT INTO t VALUES (1),(2)")
	res, err := p.Run(context.Background(), "exec", map[string]any{"sql": "UPDATE t SET x=9"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.Value.(map[string]any)
	if m["rowsAffected"].(int64) != 2 {
		t.Fatalf("rowsAffected=%v", m["rowsAffected"])
	}
}

func TestUnknownVerb(t *testing.T) {
	p := newTestProvider(t)
	if _, err := p.Run(context.Background(), "nope", nil); err == nil {
		t.Fatal("want error for unknown verb")
	}
}

func TestPing(t *testing.T) {
	p := newTestProvider(t)
	res, err := p.Run(context.Background(), "ping", nil)
	if err != nil || res.Value != true {
		t.Fatalf("res=%v err=%v", res, err)
	}
}
