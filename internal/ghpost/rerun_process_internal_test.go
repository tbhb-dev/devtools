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
	mu sync.Mutex
}

func (l *rerunProcessLock) Lock() {
	if runtime.GOOS == "windows" {
		l.mu.Lock()
	}
}

func (l *rerunProcessLock) Unlock() {
	if runtime.GOOS == "windows" {
		l.mu.Unlock()
	}
}

func TestRerunProcessLock(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		var lock rerunProcessLock
		lock.mu.Lock()
		defer lock.mu.Unlock()
		lock.Lock()
		require.False(t, lock.mu.TryLock())
		lock.Unlock()
		require.False(t, lock.mu.TryLock())
		return
	}
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
