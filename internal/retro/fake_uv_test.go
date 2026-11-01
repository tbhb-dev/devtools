// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package retro_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	fakeUVEnv  = "RETRO_FAKE_UV"
	fakeGitEnv = "RETRO_FAKE_GIT"
)

// TestMain runs the test binary as uv or as a failing git when a fixture links it to
// that name and sets the matching fake variable. The command under test still starts a child process.
func TestMain(m *testing.M) {
	if os.Getenv(fakeGitEnv) == "1" {
		_, _ = fmt.Fprintln(os.Stdout, "partial path")
		os.Exit(9)
	}
	if os.Getenv(fakeUVEnv) == "1" {
		os.Exit(fakeUV(os.Args[1:], os.Stdout))
	}
	os.Exit(m.Run())
}

// fakeUV replaces the report script on hosts without uv.
// It verifies the environment and arguments the command passes and prints canned reports.
func fakeUV(args []string, stdout io.Writer) int {
	if len(args) != 7 || args[0] != "run" || args[1] != "--script" || args[4] != "--format" {
		return 2
	}
	format := os.Getenv("ANALYZE_SESSION_FORMAT")
	if format == "" {
		format = "auto"
	}
	if os.Getenv("LC_ALL") != "C" || os.Getenv("PYTHONUTF8") != "1" || args[5] != format {
		return 3
	}
	report, transcript := args[3], args[6]
	if report == os.Getenv("FAIL_REPORT") {
		_, _ = fmt.Fprintln(stdout, "partial output")
		return 9
	}
	if override := os.Getenv("RETRO_FAKE_META"); report == "meta" && override != "" {
		_, _ = fmt.Fprintln(stdout, override)
		return 0
	}
	if report == "meta" {
		id := fakeSessionID(transcript)
		_, _ = fmt.Fprintf(stdout, "SESSION_ID=%s\nSESSION_DATE=20260102\nRECORDS=40\nSIZE_MB=1\n", id)
		return 0
	}
	for i := range 40 {
		_, _ = fmt.Fprintln(stdout, i)
	}
	return 0
}

// fakeSessionID names a session the way the report script does: a Codex
// transcript carries its identifier in the first record and any other file uses its stem.
func fakeSessionID(transcript string) string {
	if root, err := os.OpenRoot(filepath.Dir(transcript)); err == nil {
		defer root.Close()
		if file, openErr := root.Open(filepath.Base(transcript)); openErr == nil {
			defer file.Close()
			var first struct {
				Type    string `json:"type"`
				Payload struct {
					ID string `json:"id"`
				} `json:"payload"`
			}
			scanner := bufio.NewScanner(file)
			if scanner.Scan() && json.Unmarshal(scanner.Bytes(), &first) == nil && first.Type == "session_meta" {
				return first.Payload.ID
			}
		}
	}
	return strings.TrimSuffix(filepath.Base(transcript), ".jsonl")
}

func linkFakeUV(t *testing.T) {
	t.Helper()
	linkFake(t, "uv")
	t.Setenv(fakeUVEnv, "1")
}

// linkFake puts a link to the test binary first on PATH under the program name.
func linkFake(t *testing.T, program string) {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	bin := t.TempDir()
	if runtime.GOOS == "windows" {
		program += ".exe"
	}
	require.NoError(t, os.Link(self, filepath.Join(bin, program)))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}
