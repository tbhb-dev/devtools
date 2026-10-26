//go:build unix

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package pins_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func compareShell(t *testing.T, root string, got result) {
	t.Helper()
	script, err := filepath.Abs("../../.devtools/scripts/check-pins-legacy.sh")
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), "bash", script)
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), "LC_ALL=C")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	status := 0
	if err = cmd.Run(); err != nil {
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit)
		status = exit.ExitCode()
	}
	require.Equal(t, result{status, out.String(), stderr.String()}, got)
}
