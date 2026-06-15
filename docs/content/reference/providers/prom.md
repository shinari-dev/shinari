---
title: prom
description: "Scrape a Prometheus/OpenMetrics endpoint and select one sample for SLO assertions."
weight: 60
---

Metrics scrape. Scrapes a Prometheus/OpenMetrics endpoint and selects one
sample by metric name and labels, for asserting on the system's own SLO metrics.

```yaml
providers:
  metrics:
    source: prom
    config:
      baseUrl: http://localhost:9090
```

| verb | kind | args |
|---|---|---|
| `scrape` | probe | `metric` (primary), `path?` (default `/metrics`), `labels?` (map) |

Returns the matched sample's value as a number; errors if no line matches the
metric and labels. Selection is a direct lookup by name + label subset (no
histogram-bucket math, since the endpoint exposes the quantile):

```yaml
- run: metrics.scrape
  with: { metric: http_request_duration_seconds, labels: { quantile: "0.99" } }
  as: p99
- run: assert
  with: { of: "${.p99.value}", lt: 0.2 }
```
