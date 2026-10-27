// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func metadataGit(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	data, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", data)
	return strings.TrimSpace(string(data))
}

func metadataFixture(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_DATE", "2025-01-02T03:04:05+05:00")
	t.Setenv("GIT_COMMITTER_DATE", "2025-01-02T03:04:05+05:00")
	t.Setenv("DEVTOOLS_GO_MODULE", "example.com/consumer")
	t.Setenv("DEVTOOLS_GO_BUILDMETA", "")
	t.Setenv("DEVTOOLS_GO_VERSION", "")
	t.Chdir(t.TempDir())
	metadataGit(t, "init", "--quiet")
	metadataGit(
		t,
		"-c",
		"user.name=Fixture",
		"-c",
		"user.email=fixture@example.com",
		"-c",
		"commit.gpgSign=false",
		"commit",
		"--quiet",
		"--allow-empty",
		"-m",
		"fixture",
	)
}

func metadataFlags(t *testing.T) string {
	t.Helper()
	var out, stderr bytes.Buffer
	require.Zero(t, agentrepo.Run([]string{"go-ldflags"}, nil, &out, &stderr), "%s", &stderr)
	require.Empty(t, stderr.String())
	return out.String()
}

func TestGoLdflagsDefaults(t *testing.T) {
	metadataFixture(t)
	hash := metadataGit(t, "rev-parse", "--short=7", "HEAD")
	expected := func(version, pkg string) string {
		return fmt.Sprintf(
			"-s -w -buildid= -X %s.Version=%s -X %s.Commit=%s -X %s.Date=2025-01-01T22:04:05Z\n",
			pkg,
			version,
			pkg,
			hash,
			pkg,
		)
	}
	pkg := "example.com/consumer/internal/buildmeta"
	require.Equal(t, expected(hash, pkg), metadataFlags(t), "AC1 AC4 AC5 AC6: untagged")
	metadataGit(t, "tag", "v1.2.3")
	require.Equal(t, expected("v1.2.3", pkg), metadataFlags(t), "AC1: tagged")
	for _, version := range []string{"", "fixed+release"} {
		t.Setenv("DEVTOOLS_GO_VERSION", version)
		want := version
		if want == "" {
			want = "v1.2.3"
		}
		require.Equal(t, expected(want, pkg), metadataFlags(t), "AC2: only Version changes")
	}
	t.Setenv("DEVTOOLS_GO_VERSION", "")
	t.Setenv("DEVTOOLS_GO_BUILDMETA", "other/meta")
	require.Equal(t, expected("v1.2.3", "other/meta"), metadataFlags(t), "AC3: buildmeta override")
	t.Setenv("DEVTOOLS_GO_BUILDMETA", "")
	t.Setenv("DEVTOOLS_GO_MODULE", "other/module")
	require.Equal(t, expected("v1.2.3", "other/module/internal/buildmeta"), metadataFlags(t), "AC3: module override")
	t.Setenv("DEVTOOLS_GO_MODULE", "")
	require.NoError(t, os.WriteFile("go.mod", []byte("module discovered/module\n"), 0o600))
	require.Equal(t, expected("v1.2.3", "discovered/module/internal/buildmeta"), metadataFlags(t), "AC3: discovery")
}

func TestGoLdflagsDeterministic(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	metadataFixture(t)
	metadataGit(t, "tag", "v1.2.3")
	metadataGit(
		t,
		"-c",
		"user.name=Fixture",
		"-c",
		"user.email=fixture@example.com",
		"-c",
		"commit.gpgSign=false",
		"commit",
		"--quiet",
		"--allow-empty",
		"-m",
		"next",
	)
	want := metadataFlags(t)
	for _, abbrev := range []string{"12", "auto"} {
		metadataGit(t, "config", "core.abbrev", abbrev)
		metadataGit(t, "config", "log.showSignature", "true")
		for _, zone := range []string{"UTC", "Pacific/Honolulu"} {
			t.Setenv("TZ", zone)
			t.Setenv("LC_ALL", "C.UTF-8")
			require.Equal(t, want, metadataFlags(t), "AC8: stable complete output")
			require.Equal(t, want, metadataFlags(t))
		}
	}
}

func TestGoLdflagsNoGit(t *testing.T) {
	metadataFixture(t)
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "missing"))
	require.Equal(
		t,
		"-s -w -buildid= -X example.com/consumer/internal/buildmeta.Version=DEV -X example.com/consumer/internal/buildmeta.Commit= -X example.com/consumer/internal/buildmeta.Date=unknown\n",
		metadataFlags(t),
		"AC1 AC4 AC5 AC6: unavailable Git",
	)
}

func TestGoLdflagsDiscoveryFailure(t *testing.T) {
	metadataFixture(t)
	t.Setenv("DEVTOOLS_GO_MODULE", "")
	require.NoError(t, os.WriteFile("go.mod", []byte("invalid module declaration\n"), 0o600))
	var out, stderr bytes.Buffer
	require.NotZero(t, agentrepo.Run([]string{"go-ldflags"}, nil, &out, &stderr), "AC7")
	require.Empty(t, out.String())
	require.Contains(t, stderr.String(), "go list -m")
	t.Setenv("DEVTOOLS_GO_BUILDMETA", "other/meta")
	out.Reset()
	stderr.Reset()
	require.NotZero(
		t,
		agentrepo.Run([]string{"go-ldflags"}, nil, &out, &stderr),
		"AC3: buildmeta override does not skip module discovery",
	)
	require.Empty(t, out.String())
}
