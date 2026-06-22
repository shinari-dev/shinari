---
title: exec
description: "The escape hatch: run an arbitrary shell command from the project root."
weight: 70
---

The escape hatch. Runs a shell command from the project root, for the gaps the
typed providers do not cover: a CLI tool with no verb, a fault injected with
`tc`/`iptables`, a one-off readiness script. Reach for a typed provider first
and use `exec` only when nothing else fits.

```yaml
providers:
  exec: {}        # optional config: dir (defaults to the project root)
```

`dir` sets the working directory for every command; relative paths resolve
against the project root.

## Verbs

### run (action)

Runs `sh -c <cmd>` and captures its output. The default `kind` is `action`;
mark a read-only command `kind: probe` on the step so it re-runs during
steadyState recovery. It is also a polymorphic fault verb: set `effect: outage`
or `effect: degradation` on the step when the command injects a fault (`tc qdisc
add`, `iptables -A`), so the engine tracks it and applies the recovery check.

| arg | type | req | description |
|---|---|---|---|
| `cmd` | string | yes | the command, run as `sh -c <cmd>` (primary) |
| `env` | map | no | extra environment variables for the process |
| `dir` | string | no | working directory for this call, overriding the config `dir` |

**Returns** the trimmed stdout as the value, or the decoded structure when
stdout parses as JSON. `output` carries the combined stdout and stderr. A
non-zero exit is a step failure with stderr in the message. `meta` is empty.

```yaml
- run: exec.run
  with: "kafka-topics.sh --bootstrap-server localhost:9092 --list"
  read: "split(\"\n\") | length"
  as: topics
- run: assert
  with: { of: "${.outputs.topics.value}", gt: 0 }

- run: exec.run                     # a fault injected through the escape hatch
  with: "tc qdisc add dev eth0 root netem delay 200ms"
  effect: degradation
```
