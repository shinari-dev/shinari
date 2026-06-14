// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package sqlp is the sql built-in provider: run real SQL against the system
// under test and return structured rows scenarios can assert on.
package sqlp

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
	_ "modernc.org/sqlite"             // registers the "sqlite" driver

	"github.com/shinari-dev/shinari/sdk"
	"github.com/shinari-dev/shinari/utils/conv"
)

// driverFor maps the user-facing driver name to the registered database/sql
// driver name.
var driverFor = map[string]string{
	"sqlite":   "sqlite",
	"postgres": "pgx",
}

type Provider struct {
	db *sql.DB
}

func init() { sdk.Register("sql", New) }

func New() sdk.Provider { return &Provider{} }

func (p *Provider) Type() string { return "sql" }

// Configure validates the driver and opens a pool. sql.Open does not connect;
// the real connection is lazy on the first verb, after setup has brought the
// database up. Configure must not ping.
func (p *Provider) Configure(cfg map[string]any) error {
	drv, _ := cfg["driver"].(string)
	name, ok := driverFor[drv]
	if !ok {
		return fmt.Errorf("sql: unknown driver %q (one of: postgres, sqlite)", drv)
	}
	dsn, _ := cfg["dsn"].(string)
	if dsn == "" {
		dsn, _ = cfg["url"].(string)
	}
	if dsn == "" {
		return fmt.Errorf("sql: dsn is required")
	}
	db, err := sql.Open(name, dsn)
	if err != nil {
		return fmt.Errorf("sql: open %s: %w", drv, err)
	}
	p.db = db
	return nil
}

func (p *Provider) Verbs() []sdk.VerbSpec {
	rw := []sdk.ArgSpec{
		{Name: "sql", Type: "string", Required: true},
		{Name: "args", Type: "list"},
	}
	return []sdk.VerbSpec{
		{Name: "query", Kind: sdk.KindProbe, SideEffects: false, Primary: "sql", Args: rw},
		{Name: "exec", Kind: sdk.KindAction, SideEffects: true, Primary: "sql", Args: rw},
		{Name: "ping", Kind: sdk.KindProbe, SideEffects: false},
	}
}

func (p *Provider) Run(ctx context.Context, verb string, args map[string]any) (sdk.VerbResult, error) {
	switch verb {
	case "query":
		q, _ := args["sql"].(string)
		rows, err := p.db.QueryContext(ctx, q, params(args)...)
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("sql.query %s: %w", conv.Truncate(q, 120), err)
		}
		defer rows.Close()
		out, text, err := scanRows(rows)
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("sql.query %s: %w", conv.Truncate(q, 120), err)
		}
		return sdk.VerbResult{Value: out, Output: text}, nil
	case "exec":
		q, _ := args["sql"].(string)
		res, err := p.db.ExecContext(ctx, q, params(args)...)
		if err != nil {
			return sdk.VerbResult{}, fmt.Errorf("sql.exec %s: %w", conv.Truncate(q, 120), err)
		}
		ra, _ := res.RowsAffected()
		li, _ := res.LastInsertId()
		return sdk.VerbResult{
			Value:  map[string]any{"rowsAffected": ra, "lastInsertId": li},
			Output: fmt.Sprintf("rowsAffected=%d lastInsertId=%d", ra, li),
		}, nil
	case "ping":
		if err := p.db.PingContext(ctx); err != nil {
			return sdk.VerbResult{}, fmt.Errorf("sql.ping: %w", err)
		}
		return sdk.VerbResult{Value: true, Output: "ok"}, nil
	default:
		return sdk.VerbResult{}, fmt.Errorf("sql has no verb %q", verb)
	}
}

// params returns the positional bind parameters from the args list.
func params(args map[string]any) []any {
	raw, _ := args["args"].([]any)
	return raw
}

// normalize coerces driver-returned values into types interpolation and
// assert understand: text columns can arrive as []byte.
func normalize(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// scanRows reads all rows into a slice of column->value maps plus a tab table
// for diagnostics.
func scanRows(rows *sql.Rows) ([]map[string]any, string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, "", err
	}
	var out []map[string]any
	var b strings.Builder
	b.WriteString(strings.Join(cols, "\t"))
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, "", err
		}
		m := make(map[string]any, len(cols))
		cells := make([]string, len(cols))
		for i, c := range cols {
			v := normalize(vals[i])
			m[c] = v
			cells[i] = conv.ToString(v)
		}
		out = append(out, m)
		b.WriteString("\n" + strings.Join(cells, "\t"))
	}
	return out, b.String(), rows.Err()
}
