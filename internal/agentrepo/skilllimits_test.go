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

func TestSkillLimitHelpDoesNotReadRepository(t *testing.T) {
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "absent"))
	for _, args := range [][]string{{"--help"}, {"check-skill-limits", "--help"}} {
		var out, stderr bytes.Buffer
		require.Zero(t, agentrepo.Run(args, nil, &out, &stderr))
		require.Contains(t, out.String(), "check-skill-limits")
		require.Empty(t, stderr.String())
	}
	var out, stderr bytes.Buffer
	require.Equal(t, 1, agentrepo.Run([]string{"check-skill-limits"}, nil, &out, &stderr))
	require.Empty(t, out.String())
	require.Contains(t, stderr.String(), "check-skill-limits:")
}
