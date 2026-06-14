---
title: Built-in providers
description: Config and verb tables for docker, toxiproxy, net, http, and exec.
weight: 50
---

Five providers compile into the binary — zero install. They split by
**injection mechanism**: process control (`docker`), a proxy in the request
path (`toxiproxy`), the DNS resolver (`net`), plus two primitives (`http`,
`exec`).

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
| `logs` | probe | `service` | container log text (gate on it with `wait_until`) |

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
| `get` | probe | `path` (primary), `headers?` |
| `post` / `put` / `delete` | action | `path` (primary), `body?` (JSON), `form?` (urlencoded), `headers?` |

JSON responses decode into structured values — `read:`/`capture:` jq
expressions work on them directly. Status ≥ 400 is a step failure.

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
