---
title: Break DNS resolution
description: Make a hostname disappear, point it somewhere else, or blackhole it with the net provider.
weight: 20
---

**Goal:** simulate DNS-level failures — the outage class that bypasses every
proxy because the client never connects at all.

## Prerequisites

The `net` provider drives **dnsmasq** through conf snippets: it writes one
file per host into a directory dnsmasq watches (`conf-dir=`), then runs your
reload command. Your stack must resolve through that dnsmasq.

```yaml
providers:
  net:
    config:
      confDir: assets/dnsmasq.d
      reloadCmd: docker compose -p chaos kill -s SIGHUP dnsmasq
```

## Make a host vanish (NXDOMAIN)

```yaml
- run: net.nxdomain
  with: api.partner.com
```

The host resolves to *nothing* — the "their domain expired" failure.

## Repoint a host

```yaml
- run: net.set_dns
  with:
    host: db.internal
    ip: 10.0.0.99
```

Useful for "the failover DNS record was wrong" scenarios.

## Blackhole a host

```yaml
- run: net.dns_blackhole
  with: api.partner.com
```

Resolves to `0.0.0.0`: lookups succeed, connections go nowhere — clients hit
connect timeouts instead of resolution errors. Different failure, different
code path in your retry logic. Test both.

## Restore in teardown

Snippets are files; remove them and reload:

```yaml
teardown:
  - run: exec.run
    with: "rm -f assets/dnsmasq.d/shinari-*.conf"
  - run: exec.run
    with: "docker compose -p chaos kill -s SIGHUP dnsmasq"
  - run: docker.down
```
