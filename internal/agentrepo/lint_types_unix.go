//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import "golang.org/x/sys/unix"

// Replace the default with an access check for the caller, matching Bash.
//
//nolint:gochecknoinits // Select the access check before any lookup runs.
func init() {
	lintTypesExecutable = func(candidate string) bool {
		return unix.Access(candidate, unix.X_OK) == nil
	}
}
