---
title: Project & discovery
description: The kind Project resource, the providers block, the lock file, and how files are discovered.
weight: 80
---

## Discovery

Shinari is pointed at a directory (`--project`/`-p`, default cwd). It finds the
`kind: Project` resource to anchor the root, then **walks the whole tree**,
parsing every `.yml`/`.yaml` and collecting resources by `kind`.

- A file is a resource **iff** it carries `apiVersion: shinari/v1` and a known
  `kind`. Compose files, CI configs, and other YAML are ignored.
- A recognized header with a malformed body is an **error**, not a skip.
- Exactly one `kind: Project` per tree; two is an error.
- Filenames and nesting are free. A scenario's **suite** is its directory
  (`scenarios/<suite>/...` by convention).

## The Project resource

```yaml
apiVersion: shinari/v1
kind: Project
name: chaos
description: <text>

vars:                            # defaults; scenario vars merge over these
  sleepSecs: 30

env:                             # declared environment variables (allowlist)
  DATABASE_URL:                  # required: run ERRORs (exit 2) if unset
  PORT: 8080                     # default 8080, overridden by $PORT when set

providers:
  docker:
    config:
      composeFiles: [assets/stack.yml]
      project: chaos-run
  toxiproxy:
    config:
      adminUrl: http://localhost:8474
  net:
    config:
      resolver: dnsmasq
  app:
    use: ./providers/app
    config:
      apiBase: http://localhost:8080

output:                          # where reports go, which exporters run
  exporters:
    otlp:
      enabled: true
      endpoint: 127.0.0.1:4317
```

## The env block

A `kind: Project` may declare an `env:` block, shaped like `vars:`. It is the
project's **allowlist** of environment variables: a scenario reads one as
`${.env.NAME}`, and referencing a name the block does not declare is a `validate`
error (rule 10). Environment is project-level only; scenarios do not declare it.

Each key's value is a **default**, and the process environment overrides it:

```yaml
env:
  DATABASE_URL:        # null value: required, no default
  PORT: 8080           # default 8080, used unless $PORT is set
```

- A key with a value provides a default. The matching process variable, when
  set, overrides it.
- A key with a **null** value (no default) is **required**. If the variable is
  unset at run time the run is `ERRORED` (exit code 2), the same class as a
  failed setup: the run never happened.
- The engine never reads the process environment itself. The CLI resolves the
  `env:` block (applying defaults and overrides, enforcing required keys) and
  passes the resolved values to the engine, which exposes them under the `.env`
  namespace.

### The .env file

Like docker compose, Shinari reads a `.env` file next to the project when one is
present. It is a source of **values**, not a second allowlist: a `.env` file
supplies values for variables the `env:` block already declares, and keys the
block does not declare are ignored, never injected. So `${.env.NAME}` still
resolves against the declared allowlist and still fails `validate` for an
undeclared name.

Values resolve with a fixed precedence:

```text
process environment  >  .env file  >  env: default   (required if none provide a value)
```

A `.env` file is one `KEY=value` per line, parsed with the standard
[`godotenv`](https://github.com/joho/godotenv) library so the format matches
docker compose. Blank lines and `#` comments (whole-line or trailing) are
skipped, an optional `export ` prefix is stripped, single-quoted values are
literal, and double-quoted values honor `\n`-style escapes. A `${VAR}` or `$VAR`
reference expands to an **earlier key defined in the same file** (it does not
read the process environment):

```sh
DATABASE_URL=postgres://localhost/app
PORT=8080
HEALTH_URL=http://localhost:${PORT}/health  # expands to the PORT above
TOKEN="ab\ncd"     # double quotes honor escapes
LITERAL='raw $val' # single quotes are literal, no expansion
```

An absent default `.env` is a silent no-op. `run --env-file <path>` reads that
file instead of the project's `.env`; naming a file that does not exist is a
setup error (exit 2). Because a `.env` typically holds secrets, keep it out of
version control (add it to `.gitignore`).

Reference a declared variable from any `${...}`:

```yaml
- run: db.query
  with: "SELECT 1"
  desc: "connects to ${.env.DATABASE_URL}"
```

## The providers block

Each key is an **instance name** (the verb namespace). Fields:

| field | meaning |
|---|---|
| `config` | passed to the provider's `Configure`, shared by every verb |
| `use` | path to a local composed provider (`kind: Provider` file or directory) |
| `source` | provider type, when the instance name isn't the type (named instances) |
| `version` | reserved |

A scenario may carry its own `providers:` block, merged over the project's,
later wins, config maps merge shallowly.

## The output block

A `kind: Project` may declare an `output:` block: where reports are written and
which exporters run.

```yaml
output:
  dir: shinari-out                 # default; --out/-o overrides
  exporters:
    tsv:      { enabled: true }
    json:     { enabled: true }
    junit:    { enabled: true }
    journal:  { enabled: true }
    findings: { enabled: true }
    sarif:    { enabled: true }
    html:     { enabled: true }
    otlp:
      enabled: true
      endpoint: 127.0.0.1:4317     # literal host:port; --otlp overrides
      protocol: grpc               # only grpc is supported today
```

When the block is absent, the seven file exporters write to `shinari-out/` and
OTLP export is off. Listing one exporter sets only that exporter, so naming
`otlp` does not disable the others; a file exporter is turned off with
`enabled: false`.

`--out` overrides `output.dir`, and `--otlp host:port` overrides the OTLP
endpoint and forces export on, so a CI job points traces at its own collector
without editing the project. The endpoint is a literal address: it is not
interpolated.

`validate` reports an enabled OTLP exporter with no endpoint (rule 17), an
unsupported protocol (rule 18), and an unknown exporter key (rule 16). The
engine never reads this block; the CLI resolves it and drives the exporters.

## The lock file

`shinari init` writes `shinari.lock.yml` next to the project. Commit it:

```yaml
version: 1
providers:
  exec:
    kind: builtin
    version: 0.1.0
  jobstore:
    kind: local
    source: ./providers/jobstore
    checksum: sha256:b433d030...
```

Built-ins pin the engine version; local composed providers pin a content
checksum. There is no network fetch: every provider is either compiled in or
resolved from a local path.

## Best-practice layout

Convention, encouraged by docs and rendered nicely by `list`, never
required:

```text
<project>/
  project.yml
  shinari.lock.yml
  providers/        # composed providers
  scripts/          # shell for exec.run
  assets/           # compose files, dnsmasq confs, fixtures
  scenarios/<suite>/<name>.yml
```
