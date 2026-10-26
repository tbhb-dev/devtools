//go:build !unix

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package pins_test

import "testing"

// Shell parity runs on Unix. Behavior assertions run on every platform.
func compareShell(_ *testing.T, _ string, _ result) {}
