// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package durability_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

const batchSize = 100

// These benchmarks model filesystem operations, not the exporter or a power cut.
// Setup and cleanup are outside the timer. Use a fixed iteration limit and -count.
func BenchmarkHook(b *testing.B) {
	for _, durable := range []bool{false, true} {
		b.Run(fmt.Sprintf("directories=%t", durable), func(b *testing.B) {
			root := setup(b)
			samples := make([]int64, 0, b.N)
			b.ResetTimer()
			for i := range b.N {
				start := time.Now()
				hook(b, root, fmt.Sprintf("%d.json", i), durable)
				samples = append(samples, time.Since(start).Nanoseconds())
			}
			b.StopTimer()
			slices.Sort(samples)
			b.ReportMetric(float64(samples[(len(samples)*99-1)/100]), "p99-ns")
		})
	}
}

func BenchmarkFlush(b *testing.B) {
	for _, durable := range []bool{false, true} {
		b.Run(fmt.Sprintf("directories=%t", durable), func(b *testing.B) {
			root := setup(b)
			b.ResetTimer()
			for range b.N {
				b.StopTimer()
				for i := range batchSize {
					hook(b, root, fmt.Sprintf("%d.json", i), true)
				}
				b.StartTimer()
				flush(b, root, durable)
			}
			b.StopTimer()
		})
	}
}

func setup(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	for _, name := range []string{"tmp", "new", "cur", "state"} {
		check(tb, os.Mkdir(filepath.Join(root, name), 0o700))
	}
	for _, name := range []string{"tmp", "new", "cur", "state", "."} {
		syncDir(tb, filepath.Join(root, name))
	}
	return root
}

func hook(tb testing.TB, root, name string, durable bool) {
	tb.Helper()
	tmp := filepath.Join(root, "tmp", name)
	writeFile(tb, tmp, `{"v":1,"event":"Stop","ts_ms":1,"session":"sha-256-64:0123456789abcdef"}`)
	check(tb, os.Rename(tmp, filepath.Join(root, "new", name)))
	if durable {
		// Publish the destination before persisting removal of the source.
		syncDir(tb, filepath.Join(root, "new"))
		syncDir(tb, filepath.Join(root, "tmp"))
	}
}

func flush(tb testing.TB, root string, durable bool) {
	tb.Helper()
	claim := filepath.Join(root, "cur", ".claim-batch")
	batch := filepath.Join(root, "cur", "batch")
	check(tb, os.Mkdir(claim, 0o700))
	if durable {
		syncDir(tb, filepath.Join(root, "cur"))
	}
	for i := range batchSize {
		name := fmt.Sprintf("%d.json", i)
		check(tb, os.Rename(filepath.Join(root, "new", name), filepath.Join(claim, name)))
	}
	if durable {
		syncDir(tb, claim)
		syncDir(tb, filepath.Join(root, "new"))
	}
	check(tb, os.Rename(claim, batch))
	if durable {
		syncDir(tb, filepath.Join(root, "cur"))
	}
	buildAndResolve(tb, root, batch, durable)
	dispose(tb, root, batch, durable)
}

func buildAndResolve(tb testing.TB, root, batch string, durable bool) {
	tb.Helper()
	marker(tb, batch, "metrics.counted")
	state(tb, root, "metrics.json", true) // Already required by #456.
	marker(tb, batch, "counted")
	state(tb, root, "counts.json", durable)
	state(tb, root, "backoff.json", durable)
	for _, name := range []string{"traces.done", "logs.done"} {
		marker(tb, batch, name)
	}
}

func dispose(tb testing.TB, root, batch string, durable bool) {
	tb.Helper()
	marker(tb, batch, "dispose")
	for i := range batchSize {
		check(tb, os.Remove(filepath.Join(batch, fmt.Sprintf("%d.json", i))))
	}
	if durable {
		syncDir(tb, batch)
	}
	for _, name := range []string{"metrics.counted", "counted", "traces.done", "logs.done"} {
		check(tb, os.Remove(filepath.Join(batch, name)))
	}
	if durable {
		syncDir(tb, batch)
	}
	check(tb, os.Remove(filepath.Join(batch, "dispose")))
	if durable {
		syncDir(tb, batch)
	}
	check(tb, os.Remove(batch))
	if durable {
		syncDir(tb, filepath.Join(root, "cur"))
	}
}

func marker(tb testing.TB, dir, name string) {
	tb.Helper()
	writeFile(tb, filepath.Join(dir, name), "resolved")
	syncDir(tb, dir) // Markers already require this in #443 and #456.
}

func state(tb testing.TB, root, name string, durable bool) {
	tb.Helper()
	dir := filepath.Join(root, "state")
	tmp := filepath.Join(dir, name+".tmp")
	writeFile(tb, tmp, `{"version":1,"total":100}`)
	check(tb, os.Rename(tmp, filepath.Join(dir, name)))
	if durable {
		syncDir(tb, dir)
	}
}

func writeFile(tb testing.TB, name, data string) {
	tb.Helper()
	file, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	check(tb, err)
	_, err = file.WriteString(data)
	check(tb, err)
	check(tb, file.Sync())
	check(tb, file.Close())
}

func syncDir(tb testing.TB, name string) {
	tb.Helper()
	file, err := os.Open(name)
	check(tb, err)
	check(tb, file.Sync())
	check(tb, file.Close())
}

func check(tb testing.TB, err error) {
	tb.Helper()
	if err != nil {
		tb.Fatal(err)
	}
}
