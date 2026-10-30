// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package tokens_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/tokens"
)

func TestNativeCommand(t *testing.T) {
	bin := t.TempDir()
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	binary := filepath.Join(bin, "git"+suffix)
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "testdata/backend.go")
	output, buildErr := build.CombinedOutput()
	require.NoError(t, buildErr, "%s", output)
	require.NoError(t, os.Link(binary, filepath.Join(bin, "uv"+suffix)))
	require.NoError(t, os.Link(binary, filepath.Join(bin, "ant"+suffix)))
	t.Setenv("PATH", bin)
	root := t.TempDir()
	t.Setenv("TOKEN_TEST_ROOT", root)
	t.Setenv("MODEL", "generic")
	t.Setenv("CLAUDE_MODEL", "explicit")
	t.Setenv("CODEX_MODEL", "codex-model")
	t.Setenv("CODEX_ENCODING", "codex-encoding")
	var out, stderr bytes.Buffer
	for _, dir := range []string{".agents/skills/example", "custom"} {
		path := filepath.Join(root, filepath.FromSlash(dir))
		require.NoError(t, os.MkdirAll(path, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("body\n"), 0o600))
	}
	require.Equal(
		t,
		0,
		tokens.Run(t.Context(), []string{".agents/skills/example", "custom"}, &out, &stderr),
		stderr.String(),
	)
	record, err := os.ReadFile(filepath.Join(root, ".agents/skills/example/tokens.json"))
	require.NoError(t, err)
	require.Contains(t, string(record), `"model": "codex-model"`)
	require.Contains(t, string(record), `"tokenizer": "codex-encoding"`)
	record, err = os.ReadFile(filepath.Join(root, "custom/tokens.json"))
	require.NoError(t, err)
	require.Contains(t, string(record), `"model": "explicit"`)
	t.Setenv("CLAUDE_MODEL", "")
	require.Equal(t, 0, tokens.Run(t.Context(), []string{"custom"}, &out, &stderr))
	record, err = os.ReadFile(filepath.Join(root, "custom/tokens.json"))
	require.NoError(t, err)
	require.Contains(t, string(record), `"model": "generic"`)
	t.Setenv("TOKEN_TEST_FAIL", "1")
	require.NotZero(t, tokens.Run(t.Context(), []string{"custom"}, &out, &stderr))
	require.NotZero(t, tokens.Run(t.Context(), []string{".agents/skills/example"}, &out, &stderr))
	t.Setenv("TOKEN_TEST_GIT_FAIL", "1")
	require.NotZero(t, tokens.Run(t.Context(), nil, &out, &stderr))
}
