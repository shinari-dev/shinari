---
title: toxiproxy
description: "Proxy-in-path network faults: latency, packet loss, bandwidth, blackhole, and partition."
weight: 20
---

Proxy-in-path network faults. Drives the Toxiproxy admin API through the
official Go client.

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
