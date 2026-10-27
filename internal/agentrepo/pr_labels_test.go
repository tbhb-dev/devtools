// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestPRLabels(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, available, labels, diagnostic string
		code                                int
	}{
		{"known labels are silent", "enhancement\nbug", "bug\nenhancement\n", "", 0},
		{"all missing labels and available set", "enhancement\nbug", "change:test\nmissing\nbug\n", "no label named \"change:test\" in this repository\nno label named \"missing\" in this repository\nAvailable labels:\nenhancement\nbug\n", 1},
		{"empty repository", "", "bug\n", "no label named \"bug\" in this repository\nAvailable labels:\n\n", 1},
		{"literal names", "a.b\na[b]\n-feature", "a.b\na[b]\n-feature\n", "", 0},
		{"exact match", "Bug", "bug\n", "no label named \"bug\" in this repository\nAvailable labels:\nBug\n", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			code := agentrepo.Run(
				[]string{"pr-labels", "--available", tt.available}, strings.NewReader(tt.labels), &out, &stderr,
			)
			require.Equal(t, tt.code, code)
			require.Empty(t, out.String())
			require.Equal(t, tt.diagnostic, stderr.String())
		})
	}
}

func TestPRLabelsCompleteSnapshot(t *testing.T) {
	t.Parallel()
	var available strings.Builder
	for i := range 205 {
		fmt.Fprintf(&available, "label-%d\n", i+1)
	}
	var out, stderr bytes.Buffer
	require.Zero(t, agentrepo.Run(
		[]string{"pr-labels", "--available", available.String()}, strings.NewReader("label-205\n"), &out, &stderr,
	))
	require.Empty(t, out.String())
	require.Empty(t, stderr.String())
}

func TestPRLabelsUnreadableInput(t *testing.T) {
	t.Parallel()
	var out, stderr bytes.Buffer
	require.NotZero(t, agentrepo.Run([]string{"pr-labels"}, iotest.ErrReader(io.ErrUnexpectedEOF), &out, &stderr))
	require.Contains(t, stderr.String(), "read draft labels: unexpected EOF")
}
