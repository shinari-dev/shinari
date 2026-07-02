---
title: prom
description: "Scrape a Prometheus/OpenMetrics endpoint or run a PromQL instant query for SLO assertions."
weight: 60
---

Metrics observation, two ways: `scrape` reads an exposition endpoint directly
and selects one sample; `query` evaluates a PromQL expression on a Prometheus
server, which does the math (`rate`, `histogram_quantile`, aggregations) so an
assertion can gate on a derived signal no single exposition line carries.

```yaml
providers:
  metrics:
    source: prom
    config:
      baseUrl: http://localhost:9090
```

`baseUrl` is the host the endpoint is served from: the app's own metrics port
for `scrape`, a Prometheus server for `query`. For `scrape` the `/metrics` path
is the default and a per-step `path` overrides it.

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

### query (probe)

Evaluates a PromQL instant query via the server's `/api/v1/query`. The result
binds by shape: a **scalar** or **single-sample vector** binds its value
directly as a number (the shape an aggregated expression produces, and what
`assert` wants); a **multi-sample vector** binds a list of
`{ labels, value }` maps so `read:`/`capture:` can select. An empty result is
a probe failure, like a scrape miss, and so is a `matrix` (a range selector
without an aggregation): use an expression that returns an instant vector or
scalar.

| arg | type | req | description |
|---|---|---|---|
| `query` | string | yes | the PromQL expression to evaluate (primary) |

**Returns** the number (single sample) or the `{ labels, value }` list.
`output` is the raw API response. `meta` carries `resultType` (string) and
`samples` (int).

```yaml
- run: metrics.query
  with: 'sum(rate(http_errors_total{job="api"}[1m]))'
  as: errRate
- run: assert
  with: { of: "${.outputs.errRate.value}", lt: 0.01 }
```
