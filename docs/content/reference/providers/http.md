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
```

| verb | kind | args |
|---|---|---|
| `get` | probe | `path` (primary), `headers?`, `expectStatus?` |
| `post` / `put` / `delete` | action | `path` (primary), `body?` (JSON), `form?` (urlencoded), `headers?`, `expectStatus?` |

JSON responses decode into structured values, so `read:`/`capture:` jq
expressions work on them directly. The response carries `meta.status` and
`meta.bytes` (plus the engine's `meta.durationMs`), so after `as: rsp` you can
`assert of: "${.outputs.rsp.meta.status}"` and `assert of: "${.outputs.rsp.meta.durationMs}"`.
Status ≥ 400 is a step failure unless the code is listed in `expectStatus`
(e.g. `expectStatus: [200, 503]` to observe graceful degradation), in which case
it returns normally with the status in `meta`.
