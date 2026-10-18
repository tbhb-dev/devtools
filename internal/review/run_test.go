// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package review_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLinuxCoverageSources(t *testing.T) {
	t.Parallel()
	// The combined report includes Windows coverage but resolves files on Linux.
	cmd := exec.CommandContext(t.Context(), "go", "list", "-json", ".")
	cmd.Env = append(os.Environ(), "GOOS=linux")
	data, err := cmd.Output()
	require.NoError(t, err)
	var pkg struct {
		GoFiles []string `json:"GoFiles"`
	}
	require.NoError(t, json.Unmarshal(data, &pkg))
	require.Contains(t, pkg.GoFiles, "run_other.go")
}
