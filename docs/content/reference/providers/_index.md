---
title: Built-in providers
description: Config and verb tables for docker, toxiproxy, net, http, tcp, grpc, sql, prom, load, and exec.
weight: 50
params:
  code: SECTOR 03
  headline: Built-in providers
---

Ten providers compile into the binary (zero install). They split by
**injection mechanism**: process control (`docker`), a proxy in the request
path (`toxiproxy`), the DNS resolver (`net`), plus seven primitives (`http`,
`tcp`, `grpc`, `sql`, `prom`, `load`, `exec`). Two more composed providers ship
as examples on top of [Pumba](https://github.com/alexei-led/pumba).

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
