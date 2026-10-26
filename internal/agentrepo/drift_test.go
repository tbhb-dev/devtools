// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestVendoringCommands(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"vendor-check", "check-vendored"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			require.Zero(t, agentrepo.Run([]string{name, "--help"}, nil, &out, &stderr))
			require.Contains(t, out.String(), name)
			require.Empty(t, stderr.String())
			require.NotZero(t, agentrepo.Run([]string{name, "unexpected"}, nil, &out, &stderr))
		})
	}
}
