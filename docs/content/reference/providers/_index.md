---
title: Built-in providers
description: Config and verb tables for docker, toxiproxy, net, http, tcp, grpc, sql, redis, prom, load, and exec.
weight: 50
params:
  code: SECTOR 03
  headline: Built-in providers
---

Eleven providers compile into the binary (zero install). They split by
**injection mechanism**: process control (`docker`), a proxy in the request
path (`toxiproxy`), the DNS resolver (`net`), plus eight primitives (`http`,
`tcp`, `grpc`, `sql`, `redis`, `prom`, `load`, `exec`). Two more composed
providers ship as examples on top of
[Pumba](https://github.com/alexei-led/pumba).

## Named instances

The configured name is the namespace. Configure one native type twice with
`source:` to address two deployments:

```yaml
providers:
  apiA:
    source: http
    config:
      baseUrl: http://a:8080
  apiB:
    source: http
    config:
      baseUrl: http://b:8080
```

…then `apiA.get`, `apiB.get`. A composed instance (`use:`) takes no `config:`
of its own: its body's leaf verbs resolve against the native instances
configured beside it, so the native instance's config decides the target.
