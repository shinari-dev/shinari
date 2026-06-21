<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/shinari-mark-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/assets/shinari-mark-light.svg">
  <img src="docs/assets/shinari-mark-light.svg" alt="Shinari logo" width="140">
</picture>

# Shinari

**Resilience testing for every engineer.**

Chaos engineering, made deterministic. No platform, no specialists.
Just a binary and a YAML file you run on your laptop or in CI, anytime.

[![License](https://img.shields.io/badge/license-Apache--2.0-ff4f2b)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)

[Documentation](https://shinari.dev) · [Quickstart](#quickstart) · [Examples](examples/quickstart)

</div>

---

Shinari is a resilience integration testing framework. A test is a YAML scenario:
Shinari brings up a real system, hits it with controlled, deterministic faults
(kill a process, partition the network, poison DNS) and asserts how it survives.
Every run ends in a verdict, and every known weakness is tracked in a findings
ledger that keeps CI green until the gap is actually fixed.

## A crash is a test case

The whole harness is one YAML file. Write the failure you fear, and Shinari runs
it for real:

```yaml
kind: Scenario
name: checkout-survives-cache-outage

steadyState:            # only test a healthy system
  - run: http.get
    with: /health

method:
  - phase: "Kill the cache out from under the API"
    steps:
      - run: docker.kill
        with: redis
      - run: http.get   # checkout must answer without it
        with: /checkout/42
        as: rsp

  - phase: "Bring the cache back"
    steps:
      - run: docker.start
        with: redis

verify:
  - run: assert
    with: { of: "${.rsp.value.total}", equals: 19.90 }
    desc: "served from Postgres, priced correctly"
  - run: assert
    with: { of: "${.rsp.meta.durationMs}", lt: 200 }
    desc: "checkout answered under 200ms with the cache down"
    finding: "cold cache: checkout latency spikes for ~30s after restart"
```

`${.rsp.value...}` is the response payload; `${.rsp.meta.durationMs}` is the
latency Shinari measured for that call. Every capture is an Observation
envelope `{value, output, meta}`, and `${...}` is a jq expression over it.

```text
$ shinari run

=== checkout-survives-cache-outage
  ✓ steady state: http.get /health
  -- Kill the cache out from under the API
  ⚡ fault injected: docker.kill redis
  ✓ http.get /checkout/42
  -- Bring the cache back
  ✓ docker.start redis
  ✓ steady state recovered: http.get /health
  ✓ served from Postgres, priced correctly
  ◆ FINDING checkout answered under 200ms with the cache down
  => PASSED

1 scenario(s): 1 passed, 1 finding(s) held
reports: shinari-out/ (results.json, junit.xml)
exit 0, CI stays green
```

## Features

- **Findings ledger.** A `finding:` marks a check as a known, expected failure.
  When the check fails, it is recorded as `FINDING` and the run stays green,
  instead of becoming a red test the team learns to ignore. When the check starts
  passing (the gap was fixed), the run flips to `FAILED` with one message:
  promote this to a hard assertion. The suite becomes living, enforced
  documentation of how the system fails.
- **Deterministic, event-gated injection.** Faults fire on observed events
  (`wait_until` a probe sees the expected state), never on wall-clock timers.
  The fault lands at the same point in the system's lifecycle on every run,
  which is what makes findings reproducible.
- **Zero platform.** One Go binary. No agents to install, no chaos service to
  operate, no cluster prerequisite.
- **CI-native.** The exit code is the verdict, and JUnit XML, JSON, and a
  findings report land in `shinari-out/` for any CI to pick up.

| verdict | meaning | exit |
|---|---|---|
| `PASSED` | all checks pass or skip; findings still fail as expected | 0 |
| `FAILED` | a check regressed, or a finding now passes (promote it) | 1 |
| `ERRORED` | setup failed, the harness never came up | 2 |
| `INCONCLUSIVE` | steadyState failed before the faults, no baseline | 3 |

CLI usage errors exit 64 to stay distinct from verdicts.

## What you can break

Every capability is a namespaced verb (`docker.kill`, `toxiproxy.partition`,
`net.dns_fail`). These providers ship in the binary:

| provider | what it gives you |
|---|---|
| `docker` | compose lifecycle and process faults: kill, stop, pause a container mid-flight |
| `toxiproxy` | network faults: latency, blackhole, partition, bandwidth limits |
| `net` | DNS faults: poison or fail resolution for one hostname (dnsmasq) |
| `http` | probe real APIs, capture status, latency, and the response body |
| `sql` | query a database to assert state (exactly-once, no data loss) |
| `redis` | drive and probe a cache: set, get, miss-survival after an outage |
| `prom` | scrape a metrics endpoint and assert on a sample |
| `load` | generate HTTP workload and assert on its degradation percentiles |
| `exec` | run any script, the escape hatch |

Domain vocabularies are **composed providers**: YAML macros over other verbs,
written in pure YAML with zero Go (see
[`examples/quickstart/providers/jobstore.yml`](examples/quickstart/providers/jobstore.yml)).
Unprefixed language builtins round it out: `assert`, `sleep`, `wait_until`,
`background`, `stop_background`.

## Install

Install the latest release (Linux and macOS, amd64/arm64):

```sh
curl -sSL https://raw.githubusercontent.com/shinari-dev/shinari/main/scripts/install.sh | sh
```

Pin a version or change the install directory:

```sh
SHINARI_VERSION=v0.2.0 BINDIR="$HOME/.local/bin" \
  sh -c "$(curl -sSL https://raw.githubusercontent.com/shinari-dev/shinari/main/scripts/install.sh)"
```

Or download a prebuilt archive for your platform from the
[Releases page](https://github.com/shinari-dev/shinari/releases), verify it against
`checksums.txt`, extract, and put `shinari` on your `PATH`.

## Quickstart

Build from source (requires Go 1.26+):

```sh
go build -o shinari ./cli

./shinari -p examples/quickstart validate   # static checks, no run
./shinari -p examples/quickstart list       # scenarios grouped by suite
./shinari -p examples/quickstart run        # execute; exit code = verdict
```

The quickstart drives a toy job store through `exec` alone, with zero
infrastructure, and carries one real finding: recovery re-runs the whole job,
so the exactly-once assertion fails as expected and the run stays green.

## Project layout

A project is just a directory. Files are recognized by their
`apiVersion`/`kind` header, so names and nesting are free. The conventional
shape:

```
project.yml                  # kind: Project (providers, defaults, vars)
shinari.lock.yml             # pinned providers (committed)
providers/                   # composed providers (kind: Provider)
scripts/  assets/            # shell, compose files, fixtures
scenarios/<suite>/<name>.yml # kind: Scenario
```

## Documentation

The [documentation](https://shinari.dev) (source in [`docs/`](docs/)) covers:

- **[Tutorials](docs/content/tutorials)**: getting started, your first scenario, your first finding
- **[How-to guides](docs/content/how-to)**: network faults, DNS faults, composing providers, running in CI, debugging a run
- **[Reference](docs/content/reference)**: every key, verb, and report format
- **[Concepts](docs/content/concepts)**: the model behind scenarios, findings, and verdicts

## Contributing

Engine internals, contracts, and conventions live in
[DEVELOPERS.md](DEVELOPERS.md). The repository splits into `core/` (the engine
library: emits a structured result and a typed event stream, never prints,
never exits), `cli/` (rendering and exit codes), and `sdk/` (the provider
contract, the only package a provider author needs).

## License

Apache License 2.0, see [LICENSE](LICENSE).
