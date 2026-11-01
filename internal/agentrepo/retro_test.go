// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestRetroCommand(t *testing.T) {
	t.Parallel()
	var out, stderr bytes.Buffer
	require.Equal(t, 2, agentrepo.Run([]string{"retro", "new"}, nil, &out, &stderr))
	require.Empty(t, out.String())
	require.Contains(t, stderr.String(), "usage:")
	out.Reset()
	stderr.Reset()
	require.Zero(t, agentrepo.Run([]string{"retro", "new", "--help"}, nil, &out, &stderr))
	require.Contains(t, out.String(), "--skill-dir")
}
