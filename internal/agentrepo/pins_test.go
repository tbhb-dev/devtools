// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestPinsCommand(t *testing.T) {
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Chdir(t.TempDir())
	var out, stderr bytes.Buffer
	require.Equal(t, 1, agentrepo.Run([]string{"check-pins"}, nil, &out, &stderr))
	require.Contains(t, stderr.String(), "missing guard-markdown")
	out.Reset()
	stderr.Reset()
	require.Zero(t, agentrepo.Run([]string{"check-pins", "--help"}, nil, &out, &stderr))
	require.Contains(t, out.String(), "agentrepo check-pins")
	require.Empty(t, stderr.String())
}
