// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package sdk

import (
	"sort"
	"sync"
)

// The factory table lets a provider announce its type to the engine without
// the engine importing the provider — the same inversion as database/sql's
// driver registry. A provider package self-registers from its init():
//
//	func init() { sdk.Register("docker", New) }
//
// and a binary loads it by importing the package (directly or via
// providers/all). The engine resolves a configured type through Factory.
var (
	factoriesMu sync.RWMutex
	factories   = map[string]func() Provider{}
)

// Register makes a provider type available to the engine under typeName,
// normally from a provider package's init(). Registering a name that is
// already taken panics: a collision between two providers is a wiring bug we
// want to surface loudly at startup, not resolve silently by import order
// (this mirrors database/sql.Register). Tests that swap behavior keep one
// registration and vary it behind the factory.
func Register(typeName string, factory func() Provider) {
	if factory == nil {
		panic("sdk.Register: nil factory for " + typeName)
	}
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	if _, dup := factories[typeName]; dup {
		panic("sdk.Register: provider type already registered: " + typeName)
	}
	factories[typeName] = factory
}

// Factory returns the constructor registered for typeName, if any.
func Factory(typeName string) (func() Provider, bool) {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()
	f, ok := factories[typeName]
	return f, ok
}

// ProviderTypes lists the registered type names, sorted — for diagnostics.
func ProviderTypes() []string {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()
	out := make([]string, 0, len(factories))
	for k := range factories {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
