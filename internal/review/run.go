// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package review bounds a reviewer and its process group.
package review

const (
	// Timeout is distinct from the review workflow's verdict statuses.
	Timeout = 124
	// Canceled reports interruption without accepting a verdict.
	Canceled = 130
	// Failed covers launch, I/O, and reviewer exit failures.
	Failed = 1
)
