---
title: net
description: "DNS-level faults via dnsmasq: redirect, NXDOMAIN, or blackhole a hostname."
weight: 30
---

DNS-level faults. Writes dnsmasq conf snippets (one file per host) into
`confDir`, then runs `reloadCmd`.

```yaml
providers:
  net:
    config:
      confDir: assets/dnsmasq.d
      reloadCmd: "pkill -HUP dnsmasq"
```

| verb | kind | args | wrote |
|---|---|---|---|
| `set_dns` | action | `host` (primary), `ip` and/or `ips` | one `address=/host/ip` line per address |
| `nxdomain` | action | `host` | `address=/host/` (the domain vanishes) |
| `dns_blackhole` | action | `host` | `address=/host/0.0.0.0` (resolves, routes nowhere) |

`set_dns` declares the full set the name resolves to. Pass a single `ip`, or `ips` for a name
backed by several A records:

```yaml
- run: net.set_dns
  with:
    host: controllers.kestra.test
    ips: [10.0.0.1, 10.0.0.2]
```

writes both records, which dnsmasq serves together:

```
address=/controllers.kestra.test/10.0.0.1
address=/controllers.kestra.test/10.0.0.2
```

Each `set_dns` replaces the whole set, so a name's live endpoints change by restating it. `nxdomain`
is the empty set and `dns_blackhole` is a one-member unroutable set.
