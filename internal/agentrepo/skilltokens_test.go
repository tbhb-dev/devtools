// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestSkillTokensCommand(t *testing.T) {
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "missing"))
	var out, stderr bytes.Buffer
	require.Equal(t, 1, agentrepo.Run([]string{"skill-tokens", "missing"}, nil, &out, &stderr))
	require.Empty(t, out.String())
	require.Contains(t, stderr.String(), "skill-tokens:")
	out.Reset()
	stderr.Reset()
	require.Zero(t, agentrepo.Run([]string{"skill-tokens", "--help"}, nil, &out, &stderr))
	require.Contains(t, out.String(), "skill-dir")
	require.Empty(t, stderr.String())
}
