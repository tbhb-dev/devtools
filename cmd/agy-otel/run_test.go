// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func realOpen(name string, flag int, perm os.FileMode) (spoolFile, error) {
	file, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return nil, fmt.Errorf("open test spool: %w", err)
	}
	return file, nil
}

func entries(t *testing.T, state, dir string) []os.DirEntry {
	t.Helper()
	list, err := os.ReadDir(filepath.Join(state, "agy-otel", dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	require.NoError(t, err)
	return list
}

func readRecord(t *testing.T, state string) map[string]any {
	t.Helper()
	list := entries(t, state, "new")
	require.Len(t, list, 1)
	data, err := os.ReadFile(filepath.Join(state, "agy-otel", "new", list[0].Name()))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(data, &got))
	return got
}

func fixture(t *testing.T, event string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", event+".json"))
	require.NoError(t, err)
	return data
}

func TestFixtures(t *testing.T) {
	t.Parallel()
	for _, event := range []string{"SessionStart", "PreToolUse", "PostToolUse", "PreInvocation", "PostInvocation", "Stop"} {
		t.Run(event, func(t *testing.T) {
			t.Parallel()
			state := t.TempDir()
			before := time.Now().UnixMilli()
			runHook([]string{"hook", event}, bytes.NewReader(fixture(t, event)), state, realOpen)
			got := readRecord(t, state)
			assert.InDelta(t, 1, got["v"], 0)
			assert.Equal(t, event, got["event"])
			assert.GreaterOrEqual(t, got["ts_ms"], float64(before))
			assert.LessOrEqual(t, got["ts_ms"], float64(time.Now().UnixMilli()))
			assert.Equal(t, "fixture-model", got["model"])
			assert.Equal(t, "fixture-agent", got["agent"])
			assert.Equal(t, "fixture-execution", got["exec"])
			for _, key := range []string{"session", "root", "parent"} {
				assert.Regexp(t, `^sha-256-64:[0-9a-f]{16}$`, got[key])
			}
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "EXCLUDED_")
			assert.NotContains(t, string(encoded), "b2e89a91")
			assert.NotContains(t, string(encoded), "root-fixture-id")
			assert.NotContains(t, string(encoded), "parent-fixture-id")
			assert.Empty(t, entries(t, state, "tmp"))
			switch event {
			case "PreToolUse", "PostToolUse":
				assert.Equal(t, "run_command", got["tool"])
				assert.InDelta(t, len("{\n      \"CommandLine\": \"EXCLUDED_ARGS\"\n    }"), got["args_bytes"], 0)
			case "PreInvocation", "PostInvocation":
				assert.Contains(t, got, "invocation")
			case "Stop":
				assert.InDelta(t, 0, got["execution"], 0)
				assert.Equal(t, "NO_TOOL_CALL", got["termination"])
				assert.Equal(t, false, got["idle"])
			}
			if event == "PostToolUse" {
				assert.InDelta(t, len("EXCLUDED_RESULT"), got["result_bytes"], 0)
				assert.Equal(t, true, got["error"])
			}
		})
	}
}

func TestHashVector(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "sha-256-64:ba7816bf8f01cfea", *hashID(new("abc")))
	assert.Nil(t, hashID(nil))
}

func TestPresenceAndLengths(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input string
		want  map[string]any
	}{
		{`{}`, map[string]any{}},
		{
			`{"modelName":"","stepIdx":0,"fullyIdle":false,"toolCall":{"name":"","args":"é\n"},"result":"","error":null}`,
			map[string]any{"model": "", "step": float64(0), "idle": false, "tool": "", "args_bytes": float64(3), "result_bytes": float64(0), "error": false},
		},
		{
			`{"error":"","result":{"ok":true},"toolCall":{"args":{}}}`,
			map[string]any{"error": true, "result_bytes": float64(11), "args_bytes": float64(2)},
		},
	} {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			state := t.TempDir()
			runHook([]string{"hook", "Stop"}, strings.NewReader(tc.input), state, realOpen)
			got := readRecord(t, state)
			delete(got, "v")
			delete(got, "event")
			delete(got, "ts_ms")
			assert.Equal(t, tc.want, got)
		})
	}
}

func buildHook(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "agy-otel")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	output, err := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".").CombinedOutput()
	require.NoError(t, err, "%s", output)
	return binary
}

func invoke(ctx context.Context, binary, state string, input []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), "XDG_STATE_HOME="+state)
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("invoke hook: %w", err)
	}
	return output, nil
}

func TestSilentFailures(t *testing.T) {
	t.Parallel()
	binary := buildHook(t)
	for _, tc := range []struct {
		name  string
		args  []string
		input string
	}{
		{"unknown event", []string{"hook", "Other"}, `{}`},
		{"missing event", []string{"hook"}, `{}`},
		{"missing command", nil, `{}`},
		{"bad JSON", []string{"hook", "Stop"}, `{`},
		{"wrong type", []string{"hook", "Stop"}, `{"modelName":42}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := t.TempDir()
			output, err := invoke(t.Context(), binary, state, []byte(tc.input), tc.args...)
			require.NoError(t, err)
			assert.Empty(t, output)
			assert.Empty(t, entries(t, state, "new"))
		})
	}
	t.Run("read-only spool", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("Windows does not enforce Unix directory modes")
		}
		state := t.TempDir()
		root := filepath.Join(state, "agy-otel")
		require.NoError(t, os.Mkdir(root, 0o500))
		t.Cleanup(func() { require.NoError(t, os.Chmod(root, os.ModeDir|0o700)) })
		output, err := invoke(t.Context(), binary, state, []byte(`{}`), "hook", "Stop")
		require.NoError(t, err)
		assert.Empty(t, output)
		assert.Empty(t, entries(t, state, "new"))
	})
}

func TestSizeLimit(t *testing.T) {
	t.Parallel()
	binary := buildHook(t)
	prefix := `{"lastUserInput":"`
	suffix := `","modelName":"kept"}`
	exact := prefix + strings.Repeat("x", maxPayload-len(prefix)-len(suffix)) + suffix
	for _, size := range []int{maxPayload, maxPayload + 1} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			t.Parallel()
			input := exact
			if size > maxPayload {
				input = prefix + "x" + exact[len(prefix):]
			}
			state := t.TempDir()
			output, err := invoke(t.Context(), binary, state, []byte(input), "hook", "SessionStart")
			require.NoError(t, err)
			assert.Empty(t, output)
			if size == maxPayload {
				assert.Equal(t, "kept", readRecord(t, state)["model"])
			} else {
				assert.Empty(t, entries(t, state, "new"))
			}
		})
	}
}

type failingFile struct {
	*os.File

	stage string
}

func (f failingFile) Write(data []byte) (int, error) {
	if f.stage == "write" {
		// A partial write must never publish a partial record.
		if _, err := f.File.Write(data[:1]); err != nil {
			return 0, fmt.Errorf("partial test write: %w", err)
		}
		return 1, syscall.ENOSPC
	}
	n, err := f.File.Write(data)
	if err != nil {
		return n, fmt.Errorf("test write: %w", err)
	}
	return n, nil
}

func (f failingFile) Sync() error {
	if f.stage == "sync" {
		return syscall.ENOSPC
	}
	if err := f.File.Sync(); err != nil {
		return fmt.Errorf("test sync: %w", err)
	}
	return nil
}

func TestFullDisk(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"write", "sync"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			state := t.TempDir()
			open := func(name string, flag int, perm os.FileMode) (spoolFile, error) {
				file, err := os.OpenFile(name, flag, perm)
				require.NoError(t, err)
				return failingFile{File: file, stage: stage}, nil
			}
			runHook([]string{"hook", "Stop"}, strings.NewReader(`{}`), state, open)
			assert.Empty(t, entries(t, state, "new"))
			assert.Empty(t, entries(t, state, "tmp"))
		})
	}
}

func TestConcurrentReader(t *testing.T) {
	t.Parallel()
	binary := buildHook(t)
	state := t.TempDir()
	claimed := t.TempDir()
	const calls = 32
	var writers sync.WaitGroup
	failures := make(chan error, calls)
	for i := range calls {
		writers.Go(func() {
			output, err := invoke(
				t.Context(),
				binary,
				state,
				fmt.Appendf(nil, `{"executionId":"call-%d"}`, i),
				"hook",
				"PostToolUse",
			)
			if err != nil || len(output) != 0 {
				failures <- fmt.Errorf("hook failed: %s: %w", output, err)
			}
		})
	}
	done := make(chan struct{})
	go func() { writers.Wait(); close(done) }()
	seen := make(map[string]bool)
	deadline := time.After(10 * time.Second)
	for len(seen) < calls {
		for _, entry := range entries(t, state, "new") {
			path := filepath.Join(claimed, entry.Name())
			require.NoError(t, os.Rename(filepath.Join(state, "agy-otel", "new", entry.Name()), path))
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var got record
			require.NoError(t, json.Unmarshal(data, &got), "published record must be complete")
			require.NotNil(t, got.Exec)
			assert.False(t, seen[*got.Exec], "duplicate delivery")
			seen[*got.Exec] = true
		}
		select {
		case <-deadline:
			t.Fatalf("received %d of %d records", len(seen), calls)
		default:
			runtime.Gosched()
		}
	}
	<-done
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	assert.Empty(t, entries(t, state, "new"))
	assert.Empty(t, entries(t, state, "tmp"))
	for i := range calls {
		assert.True(t, seen[fmt.Sprintf("call-%d", i)])
	}
}

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

func TestDefaultStateHome(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", state)
	t.Setenv("USERPROFILE", state)
	Run([]string{"hook", "SessionStart"}, strings.NewReader(`{}`))
	assert.Len(t, entries(t, filepath.Join(state, ".local", "state"), "new"), 1)
}

func TestExplicitStateHome(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	Run([]string{"hook", "SessionStart"}, strings.NewReader(`{}`))
	assert.Len(t, entries(t, state, "new"), 1)
}

func TestUnrelatedSpoolFiles(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	for _, dir := range []string{"tmp", "new"} {
		path := filepath.Join(state, "agy-otel", dir)
		require.NoError(t, os.MkdirAll(path, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(path, "other.json"), []byte("untouched"), 0o600))
	}
	runHook([]string{"hook", "Stop"}, strings.NewReader(`{}`), state, realOpen)
	assert.Len(t, entries(t, state, "new"), 2)
	assert.Len(t, entries(t, state, "tmp"), 1)
	for _, dir := range []string{"tmp", "new"} {
		data, err := os.ReadFile(filepath.Join(state, "agy-otel", dir, "other.json"))
		require.NoError(t, err)
		assert.Equal(t, "untouched", string(data))
	}
}
