//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func metadataStub(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/bash\n"+body+"\n"), 0o600))
	//nolint:gosec // Fake commands must be executable to test the process boundary.
	require.NoError(t, os.Chmod(path, 0o700))
}

func TestGoLdflagsWrapper(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	metadataFixture(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	metadataStub(t, filepath.Join(bin, "mise"), "exit 1")
	for _, shim := range []string{".devtools/scripts/go-ldflags.sh", "tools/go-ldflags.sh"} {
		for _, mode := range []string{"capable", "old", "unsupported", "missing", "selected", "selected-capable", "failure"} {
			t.Run(shim+"/"+mode, func(t *testing.T) {
				t.Setenv("DEVTOOLS_GO_VERSION", "")
				metadataWrapperMode(t, bin, mode)
				code, out, stderr := markerShim(t, filepath.Join(root, shim), "one argument", "", "--flag")
				if mode == "capable" || mode == "selected-capable" || mode == "failure" {
					want := 0
					if mode == "failure" {
						want = 23
					}
					require.Equal(t, want, code, "AC9: operational failure must not fall back")
					require.Equal(t, "<go-ldflags>\n<one argument>\n<>\n<--flag>\n", out)
					require.Equal(t, "diagnostic\n", stderr)
				} else {
					require.Zero(t, code, "%s", stderr)
					require.Contains(t, out, ".Version=")
					require.Empty(t, stderr)
				}
			})
		}
	}
}

func metadataWrapperMode(t *testing.T, bin, mode string) {
	t.Helper()
	metadataStub(t, filepath.Join(bin, "mise"), "exit 1")
	binary := filepath.Join(bin, "agentrepo")
	body := "if [[ $* == 'go-ldflags --help' ]]; then exit 0; fi\nprintf '<%s>\\n' \"$@\"; echo diagnostic >&2"
	switch mode {
	case "old":
		body = "exit 1"
	case "unsupported":
		body = "exit 126"
	case "failure":
		body += "; exit 23"
	}
	metadataStub(t, binary, body)
	if mode == "missing" {
		require.NoError(t, os.Remove(binary))
		t.Setenv("PATH", bin+":/usr/bin:/bin")
	}
	if mode == "selected" || mode == "selected-capable" {
		selected := filepath.Join(t.TempDir(), "agentrepo")
		selectedBody := "exit 1"
		if mode == "selected-capable" {
			selectedBody = body
			metadataStub(t, binary, "exit 1")
		}
		metadataStub(t, selected, selectedBody)
		metadataStub(t, filepath.Join(bin, "mise"), "printf '%s\\n' '"+selected+"'")
	}
}

func TestGoLdflagsShellParity(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	metadataFixture(t)
	frozen := filepath.Join(root, ".devtools/scripts/legacy/go-ldflags.sh")
	for _, state := range []string{"untagged", "tagged", "missing"} {
		if state == "tagged" {
			metadataGit(t, "tag", "v1.2.3")
		}
		if state == "missing" {
			t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "missing"))
		}
		for _, pkg := range []string{"", "other/meta"} {
			t.Setenv("DEVTOOLS_GO_BUILDMETA", pkg)
			cmd := exec.CommandContext(t.Context(), "bash", frozen)
			out, outputErr := cmd.Output()
			require.NoError(t, outputErr)
			require.Equal(t, string(out), metadataFlags(t), "AC1 AC3 AC4 AC5 AC6 AC8: frozen parity %s", state)
		}
	}
	t.Setenv("DEVTOOLS_GO_VERSION", "fixed")
	cmd := exec.CommandContext(t.Context(), "bash", frozen)
	out, outputErr := cmd.Output()
	require.NoError(t, outputErr)
	require.Contains(t, string(out), ".Version=DEV", "AC9: frozen fallback keeps the old contract")
	require.Contains(t, metadataFlags(t), ".Version=fixed", "AC2: override requires the Go capability")
}

func metadataTask(t *testing.T, root, path, heading string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	require.NoError(t, err)
	section := strings.SplitN(string(data), heading, 2)
	require.Len(t, section, 2)
	body := strings.SplitN(section[1], "'''", 3)
	require.Len(t, body, 3)
	return body[1]
}

func TestGoLdflagsCallers(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	helper := filepath.Join(t.TempDir(), "agentrepo")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", helper, "./cmd/agentrepo")
	build.Dir = root
	data, err := build.CombinedOutput()
	require.NoError(t, err, "%s", data)
	metadataFixture(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("DEVTOOLS_DIR", filepath.Join(root, ".devtools"))
	t.Setenv("GOBIN", t.TempDir())
	metadataStub(t, filepath.Join(bin, "mise"), "printf '%s\\n' '"+helper+"'")
	for path, content := range map[string]string{
		"go.mod":                     "module example.com/consumer\n",
		"internal/buildmeta/meta.go": "package buildmeta\nvar Version, Commit, Date string\n",
		"cmd/show/main.go":           "package main\nimport (\"fmt\"; \"example.com/consumer/internal/buildmeta\")\nfunc main(){fmt.Printf(\"%s|%s|%s\\n\",buildmeta.Version,buildmeta.Commit,buildmeta.Date)}\n",
		"tools/go-ldflags.sh":        "exec bash '" + filepath.Join(root, "tools/go-ldflags.sh") + "' \"$@\"\n",
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	hash := metadataGit(t, "rev-parse", "--short=7", "HEAD")
	for _, caller := range []struct {
		path, heading, executable string
		args                      []string
	}{
		{".devtools/tasks/go.toml", "[\"devtools:build\"]", "bin/show", nil},
		{".devtools/tasks/go.toml", "[\"devtools:install\"]", filepath.Join(os.Getenv("GOBIN"), "show"), nil},
		{"mise.toml", "[tasks.run]", "", []string{"show"}},
	} {
		body := metadataTask(t, root, caller.path, caller.heading)
		for _, version := range []string{"fixed+release", "", "unset"} {
			t.Setenv("DEVTOOLS_GO_VERSION", version)
			want := version
			if want == "unset" {
				require.NoError(t, os.Unsetenv("DEVTOOLS_GO_VERSION"))
				want = hash
			}
			if want == "" {
				want = hash
			}
			cmd := exec.CommandContext(t.Context(), "bash")
			cmd.Args = append(cmd.Args, "-c", body, "task")
			cmd.Args = append(cmd.Args, caller.args...)
			out, runErr := cmd.CombinedOutput()
			require.NoError(t, runErr, "%s", out)
			if caller.executable != "" {
				cmd = exec.CommandContext(t.Context(), "bash", "-c", "exec \"$1\"", "fixture")
				cmd.Args = append(cmd.Args, caller.executable)
				out, runErr = cmd.CombinedOutput()
				require.NoError(t, runErr, "%s", out)
			}
			require.Equal(
				t,
				want+"|"+hash+"|2025-01-01T22:04:05Z\n",
				string(out),
				"AC2: stamped through %s",
				caller.heading,
			)
		}
		t.Setenv("DEVTOOLS_GO_MODULE", "")
		metadataStub(
			t,
			filepath.Join(bin, "go"),
			"if [[ $* == 'list -m' ]]; then echo example.com/plausible; exit 23; fi; touch compiled",
		)
		cmd := exec.CommandContext(t.Context(), "bash")
		cmd.Args = append(cmd.Args, "-c", body, "task")
		cmd.Args = append(cmd.Args, caller.args...)
		out, runErr := cmd.CombinedOutput()
		require.Error(t, runErr, "AC7: module discovery failure must stop %s", caller.heading)
		require.Contains(t, string(out), "go list -m")
		_, err = os.Stat("compiled")
		require.True(t, os.IsNotExist(err), "AC7: compilation never starts")
		require.NoError(t, os.Remove(filepath.Join(bin, "go")))
		t.Setenv("DEVTOOLS_GO_MODULE", "example.com/consumer")
	}
}
