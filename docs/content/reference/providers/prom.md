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

`baseUrl` is the host the endpoint is served from; the `/metrics` path is the
default and a per-step `path` overrides it.

## Verbs

### scrape (probe)

Fetches the exposition text and selects one sample by metric name and a label
subset. Selection is a direct lookup (no histogram-bucket math, since the
endpoint exposes the quantile), and a metric with no matching line is a probe
failure.

| arg | type | req | description |
|---|---|---|---|
| `metric` | string | yes | the metric name to select (primary) |
| `path` | string | no | scrape path (default `/metrics`) |
| `labels` | map | no | label subset the sample must match |

**Returns** the matched sample's value as a number. `output` is the raw
exposition body. `meta` is empty.

```yaml
- run: metrics.scrape
  with: { metric: http_request_duration_seconds, labels: { quantile: "0.99" } }
  as: p99
- run: assert
  with: { of: "${.outputs.p99.value}", lt: 0.2 }
```
