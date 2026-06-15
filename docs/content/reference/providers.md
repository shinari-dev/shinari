---
title: Built-in providers
description: Config and verb tables for docker, toxiproxy, net, http, sql, prom, and exec.
weight: 50
---

Seven providers compile into the binary — zero install. They split by
**injection mechanism**: process control (`docker`), a proxy in the request
path (`toxiproxy`), the DNS resolver (`net`), plus four primitives (`http`,
`sql`, `prom`, `exec`).

## docker — lifecycle + process faults

Drives the `docker compose` CLI. The lifecycle provider: implements
`up`/`down`, powers the default teardown.

```yaml
providers:
  docker:
    config:
      composeFiles: [assets/stack.yml]
      project: chaos-run
```

| verb | kind | args | effect |
|---|---|---|---|
| `up` | action | `services` (list, primary) | `compose up -d --wait` |
| `down` | action | — | `compose down -v --remove-orphans` |
| `kill` | action | `service` (primary) | SIGKILL |
| `stop` | action | `service` | SIGTERM (graceful path) |
| `start` | action | `service` | restart a stopped service |
| `pause` / `unpause` | action | `service` | freeze / thaw the process |
| `logs` | probe | `service` (primary), `tail?`, `since?` | container log text; `tail`/`since` fetch an incremental slice (gate on it with `wait_until`) |

Relative `composeFiles` paths resolve against the project root.

## toxiproxy — proxy-in-path network faults

Drives the Toxiproxy admin API through the official Go client.

```yaml
providers:
  toxiproxy:
    config:
      adminUrl: http://localhost:8474
```

| verb | kind | args | effect |
|---|---|---|---|
| `add_latency` | action | `proxy` (primary), `latencyMs`, `jitterMs?` | latency toxic, downstream |
| `packet_loss` | action | `proxy`, `toxicity?` (default 1.0) | drops data without closing connections |
| `bandwidth` | action | `proxy`, `rateKbps` | throttle |
| `blackhole` | action | `proxy` | connections hang: data dropped, socket open |
| `partition` | action | `proxy` | disable the proxy: connections fail fast |
| `reset` | action | — | remove all toxics, re-enable all proxies |

## net — DNS-level faults

Writes dnsmasq conf snippets (one file per host) into `confDir`, then runs
`reloadCmd`.

```yaml
providers:
  net:
    config:
      confDir: assets/dnsmasq.d
      reloadCmd: "pkill -HUP dnsmasq"
```

| verb | kind | args | wrote |
|---|---|---|---|
| `set_dns` | action | `host` (primary), `ip` | `address=/host/ip` |
| `nxdomain` | action | `host` | `address=/host/` — the domain vanishes |
| `dns_blackhole` | action | `host` | `address=/host/0.0.0.0` — resolves, routes nowhere |

## http — request + capture

The primitive composed domain providers build on.

```yaml
providers:
  http:
    config:
      baseUrl: http://localhost:8080   # alias: apiBase
```

| verb | kind | args |
|---|---|---|
| `get` | probe | `path` (primary), `headers?`, `expectStatus?` |
| `post` / `put` / `delete` | action | `path` (primary), `body?` (JSON), `form?` (urlencoded), `headers?`, `expectStatus?` |

JSON responses decode into structured values — `read:`/`capture:` jq
expressions work on them directly. The response carries `meta.status` and
`meta.bytes` (plus the engine's `meta.durationMs`), so after `as: rsp` you can
`assert of: "${.rsp.meta.status}"` and `assert of: "${.rsp.meta.durationMs}"`.
Status ≥ 400 is a step failure unless the code is listed in `expectStatus`
(e.g. `expectStatus: [200, 503]` to observe graceful degradation), in which case
it returns normally with the status in `meta`.

## sql — query + capture

Runs real SQL against the system under test and returns structured rows. A
native provider over `database/sql`.

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

## prom — metrics scrape

Scrapes a Prometheus/OpenMetrics endpoint and selects one sample by metric name
and labels — for asserting on the system's own SLO metrics.

```yaml
providers:
  metrics:
    source: prom
    config:
      baseUrl: http://localhost:9090
```

| verb | kind | args |
|---|---|---|
| `scrape` | probe | `metric` (primary), `path?` (default `/metrics`), `labels?` (map) |

Returns the matched sample's value as a number; errors if no line matches the
metric and labels. Selection is a direct lookup by name + label subset (no
histogram-bucket math — the endpoint exposes the quantile):

```yaml
- run: metrics.scrape
  with: { metric: http_request_duration_seconds, labels: { quantile: "0.99" } }
  as: p99
- run: assert
  with: { of: "${.p99.value}", lt: 0.2 }
```

## exec — the escape hatch

```yaml
providers:
  exec: {}        # optional config: dir (defaults to the project root)
```

| verb | kind | args |
|---|---|---|
| `run` | action (overridable per step) | `cmd` (primary), `env?` (map), `dir?` |

Runs `sh -c cmd` from the project root. Stdout that parses as JSON becomes a
structured value; otherwise the trimmed text. Non-zero exit is a failure with
stderr in the message. Mark read-only scripts `kind: probe` on the step.

## netem and resource — composed Pumba faults

Two composed providers ship as examples (`examples/faults/providers/`) that
drive [Pumba](https://github.com/alexei-led/pumba) for kernel-level network
faults and resource exhaustion. They need the `pumba` binary on PATH and
`NET_ADMIN` / privileged access on the target containers.

| provider | verbs |
|---|---|
| `netem` | `delay` (`target`, `ms`), `loss` (`target`, `percent`), `rate` (`target`, `kbps`), `clear` (`target`) |
| `resource` | `cpu` (`target`, `load`), `memory` (`target`, `bytes`), `io` (`target`, `workers`), `clear` (`target`) |

Each fault verb runs Pumba under `background` and reverts on `clear` via
`stop_background`. The `background` step declares `effect: degradation`, so the
fault is tracked and the recovery check applies. `netem` complements
`toxiproxy`: netem hits all traffic at the interface, with no proxy in the
request path.

Caveats, all confirmed against a live Docker + Pumba run:

- `target` is a **container name**, not a compose service name (or a `re2:`
  pattern). Pumba logs `no containers found` and exits 0 on a miss, so a wrong
  target fails silently. Pin the name (`container_name:` in the compose file) so
  a service-style target resolves.
- The fault is backgrounded and is not synchronized: nothing waits for Pumba to
  attach and apply the rule before the scenario proceeds. Add a `wait_until`/
  `sleep` if a step must observe the fault already in effect.
- Surviving a fault is asserted today; *observing the degradation itself* (for
  example, that latency actually rose) needs a latency-capturing probe that the
  current verb set does not provide.

## Named instances

The configured name is the namespace. Configure one type twice to address two
deployments:

```yaml
providers:
  appA:
    use: ./providers/app
    config:
      apiBase: http://a:8080
  appB:
    use: ./providers/app
    config:
      apiBase: http://b:8080
```

…then `appA.submit`, `appB.submit`. Native types use `source:` the same way.
