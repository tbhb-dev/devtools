// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/prsize"
)

func TestSizeHardRefusal(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name         string
		code, prose  int
		login        string
		bot, refused bool
	}{
		{"boundary", 800, 2000, "human", false, false},
		{"code", 801, 0, "human", false, true},
		{"prose", 0, 2001, "human", false, true},
		{"chore", 801, 0, "human", false, true},
		{"renovate app", 801, 2001, "app/renovate", true, false},
		{"renovate bot", 801, 2001, "renovate[bot]", true, false},
		{"other bot", 801, 0, "dependabot[bot]", true, true},
		{"renovate human", 801, 0, "app/renovate", false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			err := checkSizeLimit(&out, prsize.Result{Code: tt.code, Prose: tt.prose}, tt.login, tt.bot)
			require.Equal(t, tt.refused, err != nil)
			require.Contains(t, out.String(), "code SLOC")
			require.Contains(t, out.String(), "prose lines")
			if tt.refused {
				require.Contains(t, err.Error(), "split by concern, along commit boundaries, or as a stacked series")
			}
		})
	}
}
