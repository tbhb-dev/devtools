// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package reviewer

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/credentials"
)

func TestLinuxCoverageSources(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-json", ".")
	cmd.Env = credentials.ReplaceEnv(os.Environ(), "GOOS", "linux")
	data, err := cmd.Output()
	require.NoError(t, err)
	var pkg struct {
		GoFiles []string `json:"GoFiles"`
	}
	require.NoError(t, json.Unmarshal(data, &pkg))
	require.Contains(t, pkg.GoFiles, "exec_other.go")
	require.Contains(t, pkg.GoFiles, "exec_unix.go")
}

func TestExecOther(t *testing.T) {
	t.Parallel()
	binary, err := exec.LookPath("go")
	require.NoError(t, err)
	require.NoError(t, execOther(binary, []string{"gh", "version"}, os.Environ()))
	require.Error(t, execOther(t.TempDir()+"/missing", []string{"gh"}, os.Environ()))
}
