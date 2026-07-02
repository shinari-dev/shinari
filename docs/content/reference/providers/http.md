---
title: http
description: "Request and capture over HTTP, with structured JSON responses and status assertions."
weight: 40
---

Request and capture. The primitive composed domain providers build on.

```yaml
providers:
  http:
    config:
      baseUrl: http://localhost:8080   # alias: apiBase
      basicAuth: { username: admin, password: "${.env.PASSWORD}" }   # optional
```

`baseUrl` (alias `apiBase`) is prepended to each step's `path`. A step may pass
an absolute URL in `path` to override it. `basicAuth` (optional) applies HTTP
Basic credentials to every request; a per-step `basicAuth:` overrides it, and an
explicit `Authorization` header overrides both.

## Verbs

`path` is the primary arg on every verb, so the scalar shorthand `with: /health`
is the path. Every verb shares the same response envelope (see
[Response](#response)) and status rules (see [Status handling](#status-handling)).

### get (probe)

Reads a resource. A probe, so it re-runs during steadyState recovery and counts
as an observation.

| arg | type | req | description |
|---|---|---|---|
| `path` | string | yes | request path, appended to `baseUrl`; an absolute URL overrides it (primary) |
| `headers` | map | no | request headers |
| `basicAuth` | map | no | `{ username, password }`, overriding the provider-level credentials |
| `expectStatus` | list | no | status codes to tolerate instead of failing (see [Status handling](#status-handling)) |

**Returns** the response envelope: the decoded JSON (or raw string) in `value`,
the raw body in `output`, and `meta.status` / `meta.bytes`. See
[Response](#response).

```yaml
- run: http.get
  with: /health
  as: rsp
- run: assert
  with: { of: "${.outputs.rsp.meta.status}", equals: 200 }
```

### head (probe)

`get`'s cheap sibling: status and headers, no body. The reachability probe for
a large resource, where the report should never drag the payload along;
`meta.status` is all it keeps.

Same args as `get`. **Returns** the response envelope with an empty `value`
and `output`; `meta.status` carries the signal.

```yaml
- run: http.head
  with: /exports/latest.csv
  as: rsp
- run: assert
  with: { of: "${.outputs.rsp.meta.status}", equals: 200 }
```

### post / put / patch / delete (action)

Writes a resource: `patch` for a partial update, the others full-resource.
Actions, so they are skipped on `--dry-run`. The request body comes from
`body`, `raw`, or `form` (precedence `raw` → `body` → `form`); see
[Request](#request) for the full rules.

| arg | type | req | description |
|---|---|---|---|
| `path` | string | yes | request path, appended to `baseUrl`; an absolute URL overrides it (primary) |
| `body` | map | no | JSON-encoded request body |
| `raw` | string | no | verbatim request body, no encoding |
| `contentType` | string | no | overrides the `Content-Type` the body type implies |
| `form` | map | no | URL-encoded form body |
| `headers` | map | no | request headers |
| `basicAuth` | map | no | `{ username, password }`, overriding the provider-level credentials |
| `expectStatus` | list | no | status codes to tolerate instead of failing (see [Status handling](#status-handling)) |

**Returns** the same response envelope as `get`. See [Response](#response).

```yaml
- run: http.post
  with: /orders
  body: { item: "sku-42", qty: 2 }
  as: order
```

## Request

- **`body`** (map) is JSON-encoded and sent with `Content-Type: application/json`.
- **`raw`** (string) is sent verbatim, with no JSON encoding: for a YAML
  document, NDJSON, or any text payload. Pair it with `contentType:` to label it.
  `raw` takes precedence over `body` and `form`.
- **`contentType`** (string) overrides the `Content-Type` the body type implies.
- **`form`** (map) is URL-encoded and sent with `Content-Type:
  application/x-www-form-urlencoded`. Used only when no `body`/`raw` is present.
- **`headers`** (map) sets request headers and overrides the `Content-Type`
  above if you set it explicitly.
- **`basicAuth`** (map `{ username, password }`) sets HTTP Basic credentials for
  this request, overriding the provider-level `basicAuth`.

The body precedence is `raw` → `body` → `form`.

```yaml
- run: http.post
  with: /orders
  body: { item: "sku-42", qty: 2 }
  headers: { Authorization: "Bearer ${.env.TOKEN}" }
  as: order

- run: http.post                         # deploy a raw YAML document
  with: /flows
  raw: "${.outputs.flow_yaml}"
  contentType: application/x-yaml
```

## Response

When a step binds the result with `as: name`, the whole response is available
under `.outputs.name` as an envelope of three keys:

| field | what it holds |
|---|---|
| `value` | the parsed payload: the JSON-decoded structure when the response `Content-Type` contains `json` and parses, otherwise the raw body as a string |
| `output` | the raw response body as a string, always (for logs and diagnostics) |
| `meta` | `status` (int), `bytes` (response length), and `durationMs` (the engine stamps this on every call) |

```yaml
- run: http.get
  with: /orders/${.outputs.order.value.id}
  as: rsp
- run: assert
  with: { of: "${.outputs.rsp.meta.status}", equals: 200 }
- run: assert
  with: { of: "${.outputs.rsp.value.state}", equals: "confirmed" }
- run: assert
  with: { of: "${.outputs.rsp.meta.durationMs}", lt: 200 }
```

`read:` and `capture:` operate on the **payload** (`value`), so their jq
expressions address the decoded JSON directly through `.`. The envelope's other
two keys are bound as jq variables: **`$meta`** (`$meta.status`, `$meta.bytes`,
`$meta.durationMs`) and **`$output`** (the raw body). This lets a check gate on
the status code without first binding the whole envelope with `as:`. `as:` binds
the full envelope; the three compose:

```yaml
- run: http.get
  with: /orders
  read: "[.[] | select(.state == \"pending\")] | length"   # jq over value
  as: pending                                               # pending.value is the count

- run: wait_until                          # readiness that tolerates 401/403
  with:
    probe: { run: http.get, with: { path: /health, expectStatus: [200, 401, 403] } }
    read: "$meta.status"
    in: [200, 401, 403]
    timeout: 30
```

## Status handling

A status `< 400` succeeds. A status `>= 400` is a step failure (the message
carries the status and a truncated body) unless that code is listed in
`expectStatus`, in which case the step returns normally with the status in
`meta`. List the codes you want to tolerate to observe graceful degradation:

```yaml
- run: http.get
  with: /checkout
  expectStatus: [200, 503]   # 503 is an acceptable degraded response, not a failure
  as: rsp
- run: assert
  with: { of: "${.outputs.rsp.meta.status}", in: [200, 503] }
```

A request that never completes (connection refused, DNS failure, timeout) is a
step error rather than a status failure. Each request has a 30s default timeout;
a per-step `timeout:` of any value is authoritative and overrides it.
