---
title: Author scenarios with Claude Code
description: Use the bundled writing-resilience-scenario skill so Claude Code drafts and validates scenarios against the real engine contract instead of guessing.
weight: 70
---

**Goal:** write a correct scenario faster by letting Claude Code lean on a skill
that already knows the lifecycle, the step shape, and the validation rules.

## What ships with the repository

The repository includes a Claude Code skill at
`skills/writing-resilience-scenario/`. It is a distilled operating manual for
authoring: the scenario lifecycle, the step envelope, the result namespaces, the
findings ledger, and an annotated template that validates clean. It exists so
Claude Code writes scenarios against the real contract rather than reverse
engineering it from the Go source.

## Use it in this repository

Open the repository in Claude Code and the skill loads on its own. Ask for a
scenario in plain language:

```text
Add a scenario that kills the database mid-write and asserts the API recovers.
```

Claude loads the skill when the request matches, or you can invoke it directly:

```text
/writing-resilience-scenario
```

Either way it drafts a `kind: Scenario` file, wires the providers and builtins,
and runs the validate loop until the file is well formed.

## Use it on your own project

The skill is a directory, so it travels. To make it available while working on a
project outside this repository, symlink it into that project's `.claude/skills/`,
or into your personal `~/.claude/skills/` to have it everywhere:

```sh
ln -s /path/to/shinari/skills/writing-resilience-scenario \
  ~/.claude/skills/writing-resilience-scenario
```

## The authoritative detail lives here

The skill carries a condensed working set. For the full detail behind anything it
drafts, read the reference:

- [Scenario reference](/reference/scenario/) and the [step shape](/reference/step/)
- [Verbs & builtins](/reference/builtins/) and the [providers](/reference/providers/)
- [Static validation](/reference/validate/) for the rules the validate loop enforces
- [The findings ledger](/concepts/findings-ledger/) for recording a known gap

Treat a clean `shinari validate` as the definition of well formed, whether you
wrote the scenario by hand or Claude Code drafted it.
