// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

// Package all blank-imports every built-in provider so that importing it
// runs their init() self-registration. A binary loads the full built-in set
// with a single underscore import of this package; adding a new built-in
// provider means adding one line here, and nothing in core changes.
package all

import (
	_ "github.com/shinari-dev/shinari/providers/dockerp"
	_ "github.com/shinari-dev/shinari/providers/execp"
	_ "github.com/shinari-dev/shinari/providers/httpp"
	_ "github.com/shinari-dev/shinari/providers/loadp"
	_ "github.com/shinari-dev/shinari/providers/netp"
	_ "github.com/shinari-dev/shinari/providers/promp"
	_ "github.com/shinari-dev/shinari/providers/sqlp"
	_ "github.com/shinari-dev/shinari/providers/toxiproxyp"
)
