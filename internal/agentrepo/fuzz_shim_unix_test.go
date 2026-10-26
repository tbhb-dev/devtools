//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFuzzGoWrappers(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	trace := fuzzFixture(t)
	t.Setenv("FUZZ_TRACE", trace)
	t.Setenv("FUZZ_SCENARIO", "target-failure")
	t.Setenv("DEVTOOLS_DIR", filepath.Join(root, ".devtools"))
	for _, entry := range []string{".devtools/scripts/fuzz.sh", "tools/fuzz.sh"} {
		for _, mode := range []string{"capable", "old", "unsupported", "missing", "selected-old"} {
			t.Run(entry+"/"+mode, func(t *testing.T) {
				t.Setenv("FUZZ_TRACE", trace)
				fuzzWrapper(t, root, trace, entry, mode)
			})
		}
	}
}

func fuzzWrapper(t *testing.T, root, trace, entry, mode string) {
	t.Helper()
	bin := t.TempDir()
	selected := filepath.Join(bin, "selected")
	log := filepath.Join(bin, "binary-trace")
	t.Setenv("BINARY_TRACE", log)
	t.Setenv("SELECTED_BINARY", selected)
	// Isolate PATH so "missing" can't find an ambient installed agentrepo.
	for _, name := range []string{"bash", "dirname", "grep", "go"} {
		tool, err := exec.LookPath(name)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(tool, filepath.Join(bin, name)))
	}
	t.Setenv("PATH", bin)
	mise := "#!/bin/bash\nprintf '%s\\n' \"$SELECTED_BINARY\"\n"
	fuzzExecutable(t, filepath.Join(bin, "mise"), mise)
	capable := "#!/bin/bash\nprintf '<%s>' \"$@\" >> \"$BINARY_TRACE\"\nprintf '\\n' >> \"$BINARY_TRACE\"\nif [[ $* == 'fuzz-go --supported' ]]; then exit 0; fi\nprintf 'go stdout\\n'\nprintf 'go stderr\\n' >&2\nexit 23\n"
	selectedScript := capable
	if mode != "capable" {
		selectedScript = "#!/bin/bash\nexit 5\n"
	}
	if mode != "missing" {
		fuzzExecutable(t, selected, selectedScript)
	}
	// A capable PATH binary must not override the consumer's selected old binary.
	pathScript := selectedScript
	if mode == "selected-old" {
		pathScript = capable
	}
	if mode != "missing" {
		fuzzExecutable(t, filepath.Join(bin, "agentrepo"), pathScript)
	}
	require.NoError(t, os.WriteFile(trace, nil, 0o600))
	code, out, stderr := markerShim(t, filepath.Join(root, entry), "./chosen path/...", "ignored")
	require.Equal(t, 23, code)
	if mode == "capable" {
		require.Equal(t, "go stdout\n", out)
		require.Equal(t, "go stderr\n", stderr)
		calls, readErr := os.ReadFile(log)
		require.NoError(t, readErr)
		require.Equal(t, "<fuzz-go><--supported>\n<fuzz-go><./chosen path/...><ignored>\n", string(calls))
		require.Empty(t, fuzzTrace(t, trace), "Go failure must never rerun through shell")
	} else {
		require.Contains(t, out, "==> example/one FuzzFirst\n")
		require.Equal(t, "target stderr\n", stderr)
		require.NotEmpty(t, fuzzTrace(t, trace))
	}
}

func fuzzExecutable(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	info, err := os.Stat(path)
	require.NoError(t, err)
	const ownerExecute = 0o100
	require.NoError(t, os.Chmod(path, info.Mode()|ownerExecute))
}
