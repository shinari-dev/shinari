---
title: netem & resource
description: "Composed Pumba providers for kernel-level network faults and resource exhaustion."
weight: 80
---

Composed Pumba faults. Two composed providers ship as examples
(`examples/faults/providers/`) that drive
[Pumba](https://github.com/alexei-led/pumba) for kernel-level network faults
and resource exhaustion. They need the `pumba` binary on PATH and
`NET_ADMIN` / privileged access on the target containers.

| provider | verbs |
|---|---|
| `netem` | `delay` (`target`, `ms`), `loss` (`target`, `percent`), `rate` (`target`, `kbps`), `clear` (`target`) |
| `resource` | `cpu` (`target`, `load`), `memory` (`target`, `bytes`), `io` (`target`, `workers`), `clear` (`target`) |

Each fault verb runs Pumba under `background` and reverts on `clear` via
`stop_background`. The `background` step declares `effect: degradation`, so the
fault is tracked and the recovery check applies. `netem` complements
`toxiproxy`: netem hits all traffic at the interface, with no proxy in the
request path.

Caveats, all confirmed against a live Docker + Pumba run:

- `target` is a **container name**, not a compose service name (or a `re2:`
  pattern). Pumba logs `no containers found` and exits 0 on a miss, so a wrong
  target fails silently. Pin the name (`container_name:` in the compose file) so
  a service-style target resolves.
- The fault is backgrounded and is not synchronized: nothing waits for Pumba to
  attach and apply the rule before the scenario proceeds. Add a `wait_until`/
  `sleep` if a step must observe the fault already in effect.
- To *observe the degradation itself* (for example, that latency actually
  rose), assert on `${.outputs.<name>.meta.durationMs}` or run a `sample`
  window while the fault is active.
