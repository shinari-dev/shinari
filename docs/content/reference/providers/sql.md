---
title: sql
description: "Run real SQL against the system and capture structured rows, over database/sql."
weight: 50
---

Query and capture. Runs real SQL against the system under test and returns
structured rows. A native provider over `database/sql`.

```yaml
providers:
  db:
    source: sql
    config:
      driver: postgres   # postgres | sqlite | mysql
      dsn: "postgres://user:pass@localhost:5432/app?sslmode=disable"   # alias: url
```

The `dsn` format follows the chosen driver:

| driver | dsn example |
|---|---|
| `postgres` | `postgres://user:pass@localhost:5432/app?sslmode=disable` |
| `sqlite` | `file:app.db` (or `:memory:`) |
| `mysql` | `user:pass@tcp(localhost:3306)/app` |

| verb | kind | args |
|---|---|---|
| `query` | probe | `sql` (primary), `args?` (list, bind params) |
| `exec` | action | `sql` (primary), `args?` (list, bind params) |
| `ping` | probe | — |

`query` returns a list of column-to-value rows; bind values through `args:`
rather than string interpolation. `exec` returns `{rowsAffected, lastInsertId}`.
`Configure` opens the pool lazily, so the database does not need to be up until
the first verb runs (after `setup`).

```yaml
- run: db.query
  with: "SELECT count(*) AS n FROM runs WHERE job_id=$1"
  args: ["${.outputs.job}"]
  read: ".[0].n"
  as: runs
- run: assert
  with: { of: "${.outputs.runs.value}", equals: 1 }
  desc: "exactly once"
```
