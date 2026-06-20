# Scenario reference

Full catalog for writing scenarios. The authoritative source is the provider
packages under `providers/`, `core/builtins/builtins.go`, and
`core/validate/validate.go`; this is the distilled version.

## Resource headers

Every file starts with a header that decides its kind (recognized by
`apiVersion`/`kind`, not filename):

```yaml
apiVersion: shinari/v1
kind: Scenario        # or Project, or Provider
name: unique-name
description: ...
```

## Project file

`project.yml` declares the provider instances a scenario may use. The instance
key is the `run:` prefix. `source:` names the provider type (defaults to the
key). `use:` points at a composed-provider file.

```yaml
apiVersion: shinari/v1
kind: Project
name: myapp
vars:
  job: sleep-1            # referenced as ${.vars.job}
env:                      # injected environment, allowlist for ${.env.NAME}
  DATABASE_URL:           # required (null default); run errors (exit 2) if unset
  PORT: 8080              # default 8080, overridden by $PORT when set
providers:
  exec: {}
  http:
    config: { baseUrl: http://localhost:8080 }
  docker:
    config: { composeFiles: [compose.yml], project: myapp }
  db:                       # a second instance of a type
    source: sql
    config: { driver: postgres, dsn: "postgres://localhost/myapp" }
  jobstore:
    use: ./providers/jobstore   # composed provider (YAML macro)
```

`sql` drivers: `postgres`, `sqlite`. (Note: `sqlite`, not `sqlite3`.)

`env:` is project-level only and shaped like `vars:`: each value is a default,
the matching process variable overrides it, and a null value (no default) makes
the key required (run errors with exit code 2 if it is unset). It is an
allowlist: only declared names may be referenced as `${.env.NAME}`.

## Builtins (unprefixed)

| Verb | Kind | Args (`with:`) |
|---|---|---|
| `assert` | assertion | `of:` + exactly one operator (below) |
| `sleep` | action | `seconds:` (primary; `with: 2` works) |
| `wait_until` | probe | `probe:` (a step), one operator, `timeout:` (required), `interval:`, optional `read:` |
| `sample` | probe | `probe:`, `count:` and/or `duration:`, `interval:` -> result `.value` is `{count,p50,p75,p95,p99,min,max,mean}` (ms) |
| `parallel` | action | `branches:` (list of step-lists); barrier-joins |
| `repeat` | action | `times:` (required), `do:` (steps), `stopOnFail:` |
| `background` | action | `name:`, `step:` -> stop with `stop_background` |
| `stop_background` | action | `name:` (primary) |

### Assert operators (exactly one per assert/wait_until)

`equals`, `notEquals`, `contains`, `absent`, `in`, `matches` (regex),
`gt`, `lt`, `gte`, `lte`, `between` (`between: [min, max]`).

```yaml
- run: assert
  with: { of: "${.outputs.r.meta.status}", equals: 200 }
  desc: "status is 200"
- run: wait_until
  with:
    probe: { run: jobstore.status, with: "${.vars.job}" }
    equals: RUNNING
    timeout: 5
    interval: 0.1
```

## Native providers

### exec — shell escape hatch
- `run` (action, polymorphic): `with:` is the command string (primary `cmd`), or
  `{cmd, env, dir}`. JSON stdout is decoded into `.value`. Override its kind per
  step: `kind: probe` / `kind: assertion`. Declare faults via `effect:`.

### http — `config: {baseUrl}`
- `get` (probe), `post`/`put`/`delete` (action). `with: {path, body, form,
  headers, expectStatus: [..]}`. `path` is the primary; relative to `baseUrl`.
  Result: `.value` (parsed body), `.output` (raw), `.meta.status`, `.meta.bytes`.

### docker — `config: {composeFiles, project}` (the lifecycle provider)
- `up` (`with: [svc, ..]` primary `services`), `down`, `kill`/`stop`/`pause`
  (primary `service`, effect outage), `start`/`unpause`, `logs` (`{service, tail,
  since}`).

### toxiproxy — `config: {baseUrl}` (Toxiproxy admin API)
- `add_latency` (`{proxy, latencyMs, jitterMs}`, degradation)
- `bandwidth` (`{proxy, rateKbps}`, degradation)
- `packet_loss` (`{proxy, toxicity}`, outage), `blackhole` (`proxy`, outage),
  `partition` (`proxy`, outage)
- `reset` (no args) — clears all toxics; the "clear the fault" verb.

### net — DNS faults, `config: {confDir, reloadCmd}`
- `set_dns` (`{host, ip}`, degradation), `nxdomain` (`host`, outage),
  `dns_blackhole` (`host`, outage).

### sql — `source: sql`, `config: {driver, dsn}`
- `query` (probe, primary `sql`, `args: [..]`) -> `.value` is rows.
- `exec` (action, `{sql, args}`) -> `.value` `{rowsAffected, lastInsertId}`.
- `ping` (probe).

### prom — `config: {baseUrl}`
- `scrape` (probe): `{metric, path, labels}` -> `.value` is the sample float.

### load — `config: {baseUrl}`
- `run` (action): `{target, rate, duration, method, headers, body}`. `rate>=1`.
  `.value` is the same percentile stats as `sample`.

## Composed providers (kind: Provider)

A YAML macro over other verbs, zero Go. `params:` are bound as `${.params.name}`
(trailing `?` = optional). A verb is either a `do:` sequence or a single
`probe:`. A composed body may also read the project's `${.env.NAME}` allowlist
directly (ambient config like tenant or credentials) and its own earlier
`.outputs` captures; it cannot reach caller `.vars`.

```yaml
apiVersion: shinari/v1
kind: Provider
name: jobstore
verbs:
  submit:
    params: [job]
    do:
      - run: exec.run
        with: "sh scripts/jobstore.sh submit ${.params.job}"
  status:
    params: [job]
    probe:
      run: exec.run
      with: "sh scripts/jobstore.sh status ${.params.job}"
      kind: probe
```

Composed verbs inherit the strongest `effect:` of their leaves. Macro nesting is
one level only (rule 4).

## Validation rules

`./shinari -p <dir> validate` runs these (Error blocks, Warn informs):

| Rule | Checks |
|---|---|
| 2 | `with:` matches the verb's arg spec |
| 3 | `run:` resolves to a configured instance/verb (skip with `onAbsent: skip`) |
| 4 | composed-provider nesting is at most one level |
| 5 | `finding:` only on assertion-kind checks |
| 6 | a `background` capture is referenced only after `stop_background` |
| 7 | recovery-shaped scenario (outage + captured work + verify awaits it) asserts exactly-once or carries a `finding:` |
| 8 | exactly one lifecycle provider (0 = warn, >1 = error) |
| 9 | `steadyState` has no one-shot mutating action (it re-runs) |
| 10 | every `${ref}` is namespaced and resolves: `.vars.X` to a declared var, `.outputs.X` to an earlier capture in execution order, `.env.X` to a name declared in the project `env:` block |
| 11 | a `degradation` fault is observed (latency assert or `sample`) |
| 12 | no `.outputs.` reference to a capture bound only in a sibling `parallel` branch |
| 13 | `repeat`: `times >= 1`, non-empty `do:`, no `finding:` in the body, background started in the body is also stopped there |
| 14 | every `tags:` entry matches `[A-Za-z0-9_./-]+` (error); no duplicate tag (warn) |
