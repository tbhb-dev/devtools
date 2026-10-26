// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestAPMCapabilityProbe(t *testing.T) {
	t.Parallel()
	var out, stderr bytes.Buffer
	require.Equal(t, 0, agentrepo.Run([]string{"apm-sync", "--help"}, nil, &out, &stderr))
	require.Contains(t, out.String(), "apm-sync")
	require.Empty(t, stderr.String())
}
