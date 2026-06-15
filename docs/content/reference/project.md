---
title: Project & discovery
description: The kind Project resource, the providers block, the lock file, and how files are discovered.
weight: 80
---

## Discovery

Shinari is pointed at a directory (`-C`, default cwd). It finds the
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
