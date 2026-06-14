// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package main

// Load the built-in providers. Importing this package runs each provider's
// init(), which self-registers its type with the sdk factory table that the
// registry resolves against. The CLI is the composition root: it decides
// which providers the binary ships. Nothing in core links a provider.
import _ "github.com/shinari-dev/shinari/providers/all"
