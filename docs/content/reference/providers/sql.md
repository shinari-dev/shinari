---
title: sql
description: "Run real SQL against the system and capture structured rows, over database/sql."
weight: 50
---

Query and capture. Runs real SQL against the system under test and returns
structured rows. A native provider over `database/sql`. The pool opens lazily,
so the database does not need to be up until the first verb runs (after
`setup`). The database outage or latency itself is injected by the fault
providers (`net`, `toxiproxy`, `docker`); `sql` is the workload and observation
lens.

```yaml
providers:
  db:
    source: sql
    config:
      driver: postgres   # postgres | sqlite | mysql
      dsn: "postgres://user:pass@localhost:5432/app?sslmode=disable"   # alias: url
```

`driver` selects the SQL dialect; the `dsn` (alias `url`) format follows it:

| driver | dsn example |
|---|---|
| `postgres` | `postgres://user:pass@localhost:5432/app?sslmode=disable` |
| `sqlite` | `file:app.db` (or `:memory:`) |
| `mysql` | `user:pass@tcp(localhost:3306)/app` |

Bind values through `args:` rather than string interpolation; the placeholder
syntax (`$1`, `?`, …) follows the driver.

## Verbs

### query (probe)

Runs a `SELECT` and returns the rows.

| arg | type | req | description |
|---|---|---|---|
| `sql` | string | yes | the query to run (primary) |
| `args` | list | no | bind parameters, in order |

**Returns** a list of rows, each a `column -> value` map (`[]byte` columns
become strings). `output` is a tab-separated table (header row plus one line per
row). `meta` is empty.

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

### exec (action)

Runs an `INSERT`/`UPDATE`/`DELETE`/DDL statement.

| arg | type | req | description |
|---|---|---|---|
| `sql` | string | yes | the statement to run (primary) |
| `args` | list | no | bind parameters, in order |

**Returns** a map with `rowsAffected` (int) and `lastInsertId` (int). `output`
is `"rowsAffected=<n> lastInsertId=<id>"`. `meta` is empty.

```yaml
- run: db.exec
  with: "UPDATE jobs SET state='retried' WHERE id=$1"
  args: ["${.outputs.job}"]
  as: upd
- run: assert
  with: { of: "${.outputs.upd.value.rowsAffected}", equals: 1 }
```

### ping (probe)

Verifies the connection is alive. A failure is a probe failure, so `ping` works
as a steadyState gate.

No args.

**Returns** `true`. `output` is `"ok"`. `meta` is empty.

```yaml
steadyState:
  - run: db.ping
```
