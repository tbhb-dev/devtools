// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/ghpost"
)

func TestScan(t *testing.T) {
	t.Parallel()
	tests := []struct{ category, bad, good string }{
		{"file-url", "FILE:" + "///tmp/report", "https://example.com/report"},
		{"home-path", "/Users/" + "example/report", "/Users/<name>/report"},
		{"home-path", "/home/" + "example/report", "/home/<name>/report"},
		{"secret", "ghp_" + strings.Repeat("a", 36), "ghp_example"},
		{"secret", "github_pat_" + strings.Repeat("b", 50), "github_pat_example"},
		{"secret", "-----BEGIN " + "OPENSSH PRIVATE KEY-----", "a public key"},
		{"secret", "-----BEGIN " + "PGP PRIVATE KEY BLOCK-----", "PGP public key"},
		{"secret", "op:" + "//vault/item/field", "secret manager documentation"},
		{"private-string", "hidden-literal", "public-literal"},
	}
	for _, tt := range tests {
		t.Run(tt.category+tt.good, func(t *testing.T) {
			t.Parallel()
			require.Equal(
				t,
				[]ghpost.Finding{{Category: tt.category, Line: 2}},
				ghpost.Scan("safe\n"+tt.bad, []string{"hidden-literal"}, false),
			)
			require.Empty(t, ghpost.Scan(tt.good, []string{"hidden-literal"}, false))
		})
	}
	require.Empty(t, ghpost.Scan("/Users/<name>/ /home/{name}/ /home/$USER/ /home/%s/", nil, false))
	require.Empty(
		t,
		ghpost.Scan("file:"+"///tmp/report /home/"+"example/x hidden-literal", []string{"hidden-literal"}, true),
	)
	require.Equal(t, []ghpost.Finding{{Category: "secret", Line: 1}}, ghpost.Scan("op:"+"//vault/item", nil, true))
}
