//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

type rerunProcessLock struct{}

func (*rerunProcessLock) Lock()   {}
func (*rerunProcessLock) Unlock() {}
