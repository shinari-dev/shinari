// SPDX-FileCopyrightText: 2026 The Shinari Authors
// SPDX-License-Identifier: Apache-2.0

package sdk

import "testing"

func TestRegisterAndFactory(t *testing.T) {
	Register("reg-test-a", func() Provider { return nil })
	if _, ok := Factory("reg-test-a"); !ok {
		t.Fatal("registered type not found")
	}
	if _, ok := Factory("reg-test-absent"); ok {
		t.Fatal("absent type reported present")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	Register("reg-test-dup", func() Provider { return nil })
	defer func() {
		if recover() == nil {
			t.Fatal("re-registering a type must panic")
		}
	}()
	Register("reg-test-dup", func() Provider { return nil })
}

func TestRegisterNilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a nil factory must panic")
		}
	}()
	Register("reg-test-nil", nil)
}

func TestProviderTypesSorted(t *testing.T) {
	Register("reg-test-zzz", func() Provider { return nil })
	Register("reg-test-aaa", func() Provider { return nil })
	types := ProviderTypes()
	last := ""
	for _, ty := range types {
		if ty < last {
			t.Fatalf("ProviderTypes not sorted: %v", types)
		}
		last = ty
	}
}
