// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

//go:build agy_otel_latency

package main

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:paralleltest // Concurrent subprocess tests would distort the wall-time percentile.
func TestHookLatency(t *testing.T) {
	// Sequential measurements isolate each subprocess's wall time.
	binary := buildHook(t)
	state := t.TempDir()
	events := []string{"SessionStart", "PreToolUse", "PostToolUse", "PreInvocation", "PostInvocation", "Stop"}
	inputs := make([][]byte, len(events))
	for i, event := range events {
		inputs[i] = fixture(t, event)
	}
	const calls = 500
	durations := make([]time.Duration, calls)
	for i := range calls {
		index := i % len(events)
		start := time.Now()
		output, err := invoke(t.Context(), binary, state, inputs[index], "hook", events[index])
		durations[i] = time.Since(start)
		require.NoError(t, err)
		require.Empty(t, output)
	}
	require.Len(t, entries(t, state, "new"), calls)
	slices.Sort(durations)
	p99 := durations[494]
	t.Logf("500 hook subprocesses: p99 = %s", p99)
	assert.LessOrEqual(t, p99, 20*time.Millisecond)
}
