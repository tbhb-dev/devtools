//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// Windows children must not write the shared coverage metadata concurrently.
type rerunProcessLock struct {
	sync.Mutex
}

func TestRerunProcessLock(t *testing.T) {
	t.Parallel()
	var lock rerunProcessLock
	var active atomic.Int32
	counts := make(chan int32, 32)
	for range cap(counts) {
		go func() {
			lock.Lock()
			counts <- active.Add(1)
			runtime.Gosched()
			active.Add(-1)
			lock.Unlock()
		}()
	}
	for range cap(counts) {
		require.Equal(t, int32(1), <-counts)
	}
}
