---
title: grpc
description: "A standard gRPC health-check probe over the grpc.health.v1 protocol."
weight: 55
---

gRPC health. Calls the standard `grpc.health.v1` `Health/Check` RPC and reports
the serving status, so a gRPC backend gets a typed probe instead of an `exec`
escape hatch. The target is dialed lazily (no connection until the first check),
so configuring it before the service is up is fine.

```yaml
providers:
  api:
    source: grpc
    config:
      target: "localhost:50051"   # host:port
      # tls: false                # plaintext by default (the common local/CI case)
```

| verb | kind | args | returns |
|---|---|---|---|
| `health` | probe | `service` (primary, optional) | `value`: the status string; `meta.status`, `meta.rpcMs` |

`service` defaults to `""`, which checks the whole server. A `SERVING` response
passes; any other status (`NOT_SERVING`, `UNKNOWN`, `SERVICE_UNKNOWN`) is a probe
**failure** that still surfaces the status in `value` and `meta`, so `health`
works as a `steadyState` gate and the assertion can read what it saw.

```yaml
steadyState:
  - run: api.health                 # whole server must be SERVING

verify:
  - run: api.health
    with: "orders.v1.Orders"        # one named service
    as: h
  - run: assert
    with: { of: "${.outputs.h.value}", equals: SERVING }
    desc: "orders service is serving"
```
