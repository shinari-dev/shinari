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
| `set_dns` | action | `host` (primary), `ip` | `address=/host/ip` |
| `nxdomain` | action | `host` | `address=/host/` (the domain vanishes) |
| `dns_blackhole` | action | `host` | `address=/host/0.0.0.0` (resolves, routes nowhere) |
