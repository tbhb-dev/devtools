// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package metrics_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/metrics"
)

func run(t *testing.T, args ...string) {
	t.Helper()
	var output bytes.Buffer
	require.Equal(t, 0, metrics.Run(args, &output, &output), output.String())
	require.Empty(t, output.String())
}

func journal(t *testing.T, dir string) []byte {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Regexp(t, `\d{4}-\d{2}-\d{2}\.jsonl$`, files[0])
	data, err := os.ReadFile(files[0])
	require.NoError(t, err)
	return data
}

func TestEvent(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	before := time.Now().UTC()
	run(t, "event", "--name", "afakt.agent.run", "--item-ref", "item-504",
		"--attr", "repo=devtools", "--attr", "outcome=done", "--attr", "detail=a=b,c\nnext")
	data := journal(t, filepath.Join(state, "afakt", "journal"))
	require.Equal(t, 1, bytes.Count(data, []byte{'\n'}))
	var record struct {
		EventName string            `json:"event_name"`
		EventID   string            `json:"event_id"`
		Time      string            `json:"time"`
		Host      string            `json:"host"`
		ItemRef   string            `json:"item_ref"`
		Attrs     map[string]string `json:"attrs"`
	}
	require.NoError(t, json.Unmarshal(data, &record))
	compact := &bytes.Buffer{}
	require.NoError(t, json.Compact(compact, data))
	require.Equal(t, append(compact.Bytes(), '\n'), data)
	require.Equal(t, "afakt.agent.run", record.EventName)
	require.Regexp(t, `^[0-9a-f]{64}$`, record.EventID)
	require.Regexp(t, `T\d{2}:\d{2}:\d{2}\.\d{9}Z$`, record.Time)
	stamp, err := time.Parse(time.RFC3339Nano, record.Time)
	require.NoError(t, err)
	require.False(t, stamp.Before(before))
	require.False(t, stamp.After(time.Now().UTC()))
	host, err := os.Hostname()
	require.NoError(t, err)
	require.Equal(t, host, record.Host)
	require.Equal(t, "item-504", record.ItemRef)
	require.Equal(t, map[string]string{"repo": "devtools", "outcome": "done", "detail": "a=b,c\nnext"}, record.Attrs)
	id := sha256.Sum256(fmt.Appendf(nil, "%s\x00%s\x00%s\x00%s\x00%d",
		host, record.EventName, record.ItemRef, record.Time, os.Getpid()))
	require.Equal(t, hex.EncodeToString(id[:]), record.EventID)
	files, err := filepath.Glob(filepath.Join(state, "afakt", "journal", stamp.Format(time.DateOnly)+".jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
}

func TestOversizeLeavesJournalUnchanged(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	run(t, "event", "--name", "afakt.agent.run")
	dir := filepath.Join(state, "afakt", "journal")
	before := journal(t, dir)
	var output bytes.Buffer
	require.NotZero(t, metrics.Run([]string{
		"event", "--name", "afakt.agent.run",
		"--attr", "large=" + strings.Repeat("x", 5*1024),
	}, &output, &output))
	require.Contains(t, output.String(), "exceeds 4096 bytes")
	require.Equal(t, before, journal(t, dir))

	// The limit applies to JSON-escaped bytes, including the newline.
	output.Reset()
	require.NotZero(t, metrics.Run([]string{
		"event", "--name", "afakt.agent.run",
		"--attr", "large=" + strings.Repeat("\x00", 1024),
	}, &output, &output))
	require.Equal(t, before, journal(t, dir))
}

func TestConfigAndHomeFallback(t *testing.T) {
	state, home, dir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	config := filepath.Join(t.TempDir(), "metrics.toml")
	require.NoError(t, os.WriteFile(config, fmt.Appendf(nil, "journal_dir = '%s'\n", dir), 0o600))
	run(t, "event", "--config", config, "--name", "afakt.agent.run")
	data := journal(t, dir)
	require.NotContains(t, string(data), "item_ref")
	require.Contains(t, string(data), `"attrs":{}`)
	entries, err := os.ReadDir(state)
	require.NoError(t, err)
	require.Empty(t, entries)
	t.Setenv("XDG_STATE_HOME", "")
	run(t, "event", "--name", "afakt.agent.run")
	journal(t, filepath.Join(home, ".local", "state", "afakt", "journal"))
}

func TestInvalidInput(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	config := filepath.Join(t.TempDir(), "invalid.toml")
	require.NoError(t, os.WriteFile(config, []byte("journal_dir = ["), 0o600))
	for _, args := range [][]string{
		{"event"},
		{"event", "--name", "event", "--attr", "broken"},
		{"event", "--name", "event", "--attr", "=value"},
		{"event", "--name", "event", "--config", config},
		{"event", "--name", "event", "--config", config + ".missing"},
	} {
		var output bytes.Buffer
		require.NotZero(t, metrics.Run(args, &output, &output))
		require.NotEmpty(t, output.String())
	}
	entries, err := os.ReadDir(state)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func process(t *testing.T, state string) *exec.Cmd {
	t.Helper()
	binary, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), binary, "-test.run=^TestEventProcess$")
	cmd.Env = append(os.Environ(), "METRICS_TEST_PROCESS=1", "XDG_STATE_HOME="+state, "GORACE=atexit_sleep_ms=0")
	return cmd
}

func TestConcurrentProcesses(t *testing.T) {
	t.Parallel()
	const count = 20
	state := t.TempDir()
	commands := make([]*exec.Cmd, count)
	for i := range commands {
		commands[i] = process(t, state)
		require.NoError(t, commands[i].Start())
	}
	for _, cmd := range commands {
		require.NoError(t, cmd.Wait())
	}
	lines := bytes.Split(
		bytes.TrimSuffix(journal(t, filepath.Join(state, "afakt", "journal")), []byte{'\n'}),
		[]byte{'\n'},
	)
	require.Len(t, lines, count)
	ids := make(map[string]bool, count)
	for _, line := range lines {
		var record map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(line, &record))
		require.Len(t, record, 5)
		id := string(record["event_id"])
		require.NotContains(t, ids, id)
		ids[id] = true
	}
}

func TestOfflineLatency(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	cmd := process(t, state)
	start := time.Now()
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	require.NoError(t, err, string(output))
	require.Less(t, elapsed, 50*time.Millisecond)
	journal(t, filepath.Join(state, "afakt", "journal"))
	t.Logf("blocked-resolver process finished in %s", elapsed)
}

// TestEventProcess runs the command with a blocked resolver in a fresh process.
func TestEventProcess(t *testing.T) {
	t.Parallel()
	if os.Getenv("METRICS_TEST_PROCESS") != "1" {
		t.Skip("subprocess helper")
	}
	// Only this subprocess uses the resolver. Every lookup fails.
	net.DefaultResolver.PreferGo = true
	net.DefaultResolver.Dial = func(_ context.Context, _, _ string) (net.Conn, error) {
		return nil, fmt.Errorf("resolver blocked: %w", os.ErrPermission)
	}
	os.Exit(metrics.Run([]string{"event", "--name", "afakt.agent.run"}, os.Stdout, os.Stderr))
}
