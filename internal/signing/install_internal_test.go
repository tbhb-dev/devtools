// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package signing

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstall(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	binary := filepath.Join(home, "built")
	require.NoError(t, os.WriteFile(binary, []byte("test binary"), 0o600))
	c, err := ReadConfig(home, func(string) string { return "" })
	require.NoError(t, err)
	require.NoError(t, Install(home, binary, "/usr/bin/ssh-agent", c))
	data, err := os.ReadFile(filepath.Join(home, ".local/bin/ssh-sign"))
	require.NoError(t, err)
	require.Equal(t, "test binary", string(data))
	path := filepath.Join(home, ".local/share/devtools/signing")
	data, err = os.ReadFile(filepath.Join(path, "dev.tbhb.devtools.signing.plist"))
	require.NoError(t, err)
	var document any
	require.NoError(t, xml.Unmarshal(data, &document))
	require.Contains(t, string(data), "<string>-D</string>")
	require.Contains(t, string(data), filepath.Join(c.RunDir, "agent.sock"))
	if runtime.GOOS == "linux" {
		if tool, lookupErr := exec.LookPath("systemd-analyze"); lookupErr == nil {
			output, verifyErr := exec.CommandContext(t.Context(), tool, "verify", filepath.Join(path, "signing-agent.service")).
				CombinedOutput()
			require.NoError(t, verifyErr, string(output))
		}
	}
	require.Error(t, Install(home, filepath.Join(home, "missing"), "/usr/bin/ssh-agent", c))
	require.Error(t, Install(binary, binary, "/usr/bin/ssh-agent", c))
}

func TestUnitEscaping(t *testing.T) {
	t.Parallel()
	data, err := renderUnit("dev.tbhb.devtools.signing.plist", "/a&b", "/c<d")
	require.NoError(t, err)
	require.Contains(t, string(data), "/a&amp;b")
	require.Contains(t, string(data), "/c&lt;d")
	data, err = renderUnit("signing-agent.service", "/a%u$var", "/c\"d")
	require.NoError(t, err)
	require.Contains(t, string(data), "/a%%u$$var")
	require.Contains(t, string(data), `/c\"d`)
	_, err = renderUnit("missing", "", "")
	require.Error(t, err)
}
