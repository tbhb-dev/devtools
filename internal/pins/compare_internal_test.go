// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package pins

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoSites(t *testing.T) {
	t.Parallel()
	var out, stderr bytes.Buffer
	_, status := compare(nil, &out, &stderr)
	require.Equal(t, 1, status)
	require.Empty(t, out.String())
	require.Equal(
		t,
		"no tbhb-dev/devtools pins found -- check-pins verified nothing\n         run it from a repository that vendors devtools\n",
		stderr.String(),
	)
}
