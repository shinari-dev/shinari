---
title: net
description: "DNS-level faults via dnsmasq: redirect, NXDOMAIN, or blackhole a hostname, and restore it after."
weight: 30
---

DNS-level faults. Writes dnsmasq conf snippets (one file per host) into
`confDir`, then runs `reloadCmd` so the resolver picks them up.

```yaml
providers:
  net:
    config:
      confDir: assets/dnsmasq.d
      reloadCmd: "pkill -HUP dnsmasq"
```

`confDir` is where the per-host snippet files are written (relative paths
resolve against the project root); `reloadCmd` is run after each write to
reload dnsmasq.

Every fault verb returns the path of the snippet file it wrote as the value,
with the reload command's output (if any) in `output` and an empty `meta`. The
restore verbs (`clear`, `reset`) return the list of snippet paths they removed,
with `meta.removed` (int) as the count.

## Verbs

### set_dns (action)

Declares the full set of addresses a name resolves to, writing one
`address=/host/ip` line per address. Each call replaces the whole set, so a
name's live endpoints change by restating it.

| arg | type | req | description |
|---|---|---|---|
| `host` | string | yes | the hostname to redirect (primary) |
| `ip` | string | no | a single A record |
| `ips` | list | no | several A records for one name |

**Returns** the snippet file path. `output` is the reload command's output.
`meta` is empty.

```yaml
- run: net.set_dns
  with:
    host: backends.example.test
    ips: [10.0.0.1, 10.0.0.2]
```

writes both records, which dnsmasq serves together:

```
address=/backends.example.test/10.0.0.1
address=/backends.example.test/10.0.0.2
```

### nxdomain (action, outage)

Makes the name vanish: writes `address=/host/` so resolution returns NXDOMAIN.
The empty set, the opposite of `set_dns`.

| arg | type | req | description |
|---|---|---|---|
| `host` | string | yes | the hostname that should fail to resolve (primary) |

**Returns** the snippet file path. `output` is the reload command's output.
`meta` is empty.

```yaml
- run: net.nxdomain
  with: db.internal
```

### dns_blackhole (action, outage)

Resolves the name to an unroutable address: writes `address=/host/0.0.0.0`, so
lookups succeed but connections go nowhere. A one-member unroutable set, in
contrast to `nxdomain`'s empty set.

| arg | type | req | description |
|---|---|---|---|
| `host` | string | yes | the hostname to route into the void (primary) |

**Returns** the snippet file path. `output` is the reload command's output.
`meta` is empty.

```yaml
- run: net.dns_blackhole
  with: db.internal
```

### clear (action)

Lifts one host's override: removes that host's snippet file and reloads, so the
name resolves normally again. The restore side of all three fault verbs.
Idempotent — clearing a host that was never faulted (a teardown after an early
failure) removes nothing and still reloads.

| arg | type | req | description |
|---|---|---|---|
| `host` | string | yes | the hostname to restore (primary) |

**Returns** the list of removed snippet paths (empty when there was nothing to
remove). `output` is the reload command's output. `meta.removed` (int) is the
count.

```yaml
- run: net.clear
  with: db.internal
```

### reset (action)

Lifts every override this provider wrote: removes all `shinari-*.conf` snippets
in `confDir` and reloads once. Conf files it does not own are untouched. The
teardown-grade restore — one step returns DNS to its pre-scenario state no
matter which faults ran.

No args.

**Returns** the list of removed snippet paths. `output` is the reload command's
output. `meta.removed` (int) is the count.

```yaml
teardown:
  - run: net.reset
```
