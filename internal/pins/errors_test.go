// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package pins_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFailedSearchProducer(t *testing.T) {
	root := fixture(t)
	bin := t.TempDir()
	executable := filepath.Join(bin, "git")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	source := filepath.Join(bin, "git.go")
	// The only permitted subprocess is the offline, read-only tracked-text search.
	// Any attempted network or mutation command leaves evidence and fails.
	require.NoError(t, os.WriteFile(source, []byte(`package main
import ("fmt"; "os"; "strings")
func main() {
	if len(os.Args) < 2 || strings.Join(os.Args[1:], " ") != "grep -nI -o -E tbhb-dev/devtools[[:space:]]+v[0-9]+\\.[0-9]+\\.[0-9]+ --" {
	_ = os.WriteFile("forbidden-command", []byte(strings.Join(os.Args, " ")), 0600)
	os.Exit(99)
	}
	fmt.Println("README.md:1:tbhb-dev/devtools v0.10.0")
	fmt.Fprintln(os.Stderr, "search failed")
	os.Exit(23)
}
`), 0o600))
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", executable, source)
	data, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", data)
	t.Setenv("PATH", bin)
	got := check(t, root)
	require.Equal(t, 23, got.status)
	require.Contains(t, got.stderr, "search failed")
	require.NotContains(t, got.out, "every devtools pin")
	require.NoFileExists(t, filepath.Join(root, "forbidden-command"))
}

func TestDiscoveryFailure(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	// A symlink loop fails stat even when a valid pin file preceded it.
	if err := os.Symlink("apm.yml", filepath.Join(root, "apm.yml")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got := check(t, root)
	require.Equal(t, 1, got.status)
	require.Empty(t, got.out)
	require.NotEmpty(t, got.stderr)
}
