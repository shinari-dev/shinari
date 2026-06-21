---
title: Break DNS resolution
description: Make a hostname disappear, point it somewhere else, or blackhole it with the net provider.
weight: 20
---

**Goal:** simulate DNS-level failures: the outage class that bypasses every
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

The host resolves to *nothing*: the "their domain expired" failure.

## Repoint a host

```yaml
- run: net.set_dns
  with:
    host: db.internal
    ip: 10.0.0.99
```

Useful for "the failover DNS record was wrong" scenarios.

## Resolve a host to several endpoints

A service name backed by several A records uses `ips`:

```yaml
- run: net.set_dns
  with:
    host: backends.example.test
    ips: [10.0.0.1, 10.0.0.2]
```

The name now resolves to both addresses at once: the multi-endpoint discovery mode where a client
spreads across the live set and rides out the loss of any one member.

## Change the live set

`set_dns` declares the full set, so the endpoints a name resolves to change by restating it. A client
that refreshes DNS picks up an address added to the set, and stops using one dropped from it, without
restarting:

```yaml
# was {10.0.0.1, 10.0.0.2}; drop one endpoint
- run: net.set_dns
  with:
    host: backends.example.test
    ips: [10.0.0.1]
  effect: degradation
```

Restating a smaller set is the fault being injected, so the step declares `effect: degradation` (the
same per-step override `exec.run` uses when it shells out to `tc`). Restating a larger set, or the
original set, is recovery and needs no `effect:`. Taking the records away entirely is `net.nxdomain`:
a client that keeps running on its last-known endpoints through the outage, then recovers once the set
returns, is the DNS-outage resilience property.

## Blackhole a host

```yaml
- run: net.dns_blackhole
  with: api.partner.com
```

Resolves to `0.0.0.0`: lookups succeed, connections go nowhere, and clients hit
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
