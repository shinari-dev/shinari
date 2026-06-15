---
title: exec
description: "The escape hatch: run an arbitrary shell command from the project root."
weight: 70
---

The escape hatch.

```yaml
providers:
  exec: {}        # optional config: dir (defaults to the project root)
```

| verb | kind | args |
|---|---|---|
| `run` | action (overridable per step) | `cmd` (primary), `env?` (map), `dir?` |

Runs `sh -c cmd` from the project root. Stdout that parses as JSON becomes a
structured value; otherwise the trimmed text. Non-zero exit is a failure with
stderr in the message. Mark read-only scripts `kind: probe` on the step.
