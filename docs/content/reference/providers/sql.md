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
      driver: postgres   # or sqlite
      dsn: "postgres://user:pass@localhost:5432/app?sslmode=disable"   # alias: url
```

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
  args: ["${.job}"]
  read: ".[0].n"
  as: runs
- run: assert
  with: { of: "${.runs.value}", equals: 1 }
  desc: "exactly once"
```
