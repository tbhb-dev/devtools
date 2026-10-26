// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarkerBoundaries(t *testing.T) {
	t.Parallel()
	for _, marker := range []string{"<<<<<<<", ">>>>>>>", "|||||||"} {
		for _, tc := range []struct {
			prefix, suffix string
			want           int
		}{
			{"", "", 1},
			{"", " label", 1},
			{"", "\n", 1},
			{"", "text", 0},
			{"", "\tlabel", 0},
			{"", "\r\n", 0},
			{" ", "", 0},
			{"mid", " label", 0},
			{"", "<", 0},
		} {
			t.Run(marker+tc.prefix+tc.suffix, func(t *testing.T) {
				t.Parallel()
				require.Equal(t, tc.want, markerLines([]byte(tc.prefix+marker+tc.suffix)))
			})
		}
	}
	require.Zero(t, markerLines([]byte("=======\nshort\n")))
	require.Equal(t, 3, markerLines([]byte("<<<<<<< a\n=======\n||||||| base\n>>>>>>> b\n")))
	// A long preceding line mustn't hide later markers.
	require.Equal(t, 1, markerLines([]byte(strings.Repeat("x", 100_000)+"\n<<<<<<<")))
}

func TestMarkerExemptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, output string
		err          error
		want         string
	}{
		{"documented", "marked\x00conflict-markers\x00documented\x00", nil, ""},
		{"failed after documented", "marked\x00conflict-markers\x00documented\x00", errors.ErrUnsupported, "marked  1 marker line(s)\n"},
		{"other", "marked\x00conflict-markers\x00set\x00", nil, "marked  1 marker line(s)\n"},
		{"empty", "", nil, "marked  1 marker line(s)\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			scanner := markerScanner{
				attributes: func(paths []string) ([]byte, error) {
					calls++
					require.Equal(t, []string{"marked", "clean"}, paths)
					return []byte(tc.output), tc.err
				},
				read: func(path string, _ bool) ([]byte, error) {
					if path == "marked" {
						return []byte("<<<<<<<"), nil
					}
					return []byte("clean"), nil
				},
			}
			require.Equal(t, tc.want, scanner.scan([]string{"marked", "clean"}, false))
			require.Equal(t, 1, calls)
		})
	}
}

func TestMarkerSkillExemption(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"skills/resolve-rebase-conflicts/SKILL.md",
		".agents/skills/resolve-rebase-conflicts/SKILL.md",
		"/repo/.claude/skills/resolve-rebase-conflicts/SKILL.md",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			scanner := markerScanner{
				attributes: func([]string) ([]byte, error) { t.Fatal("exempt path queried"); return nil, nil },
				read:       func(string, bool) ([]byte, error) { t.Fatal("exempt path read"); return []byte("<<<<<<<"), nil },
			}
			require.Empty(t, scanner.scan([]string{path}, false))
		})
	}
	require.False(t, markerSkill("other-skills/resolve-rebase-conflicts/SKILL.md"))
}

func TestMarkerUnreadable(t *testing.T) {
	t.Parallel()
	scanner := markerScanner{
		attributes: func([]string) ([]byte, error) { return nil, nil },
		read: func(path string, staged bool) ([]byte, error) {
			if path == "unreadable" {
				return []byte("<<<<<<<"), errors.ErrUnsupported
			}
			if staged {
				return []byte("|||||||"), nil
			}
			return []byte("clean"), nil
		},
	}
	require.Empty(t, scanner.scan([]string{"unreadable", "clean"}, false))
	require.Equal(t, "marked  1 marker line(s)\n", scanner.scan([]string{"unreadable", "marked"}, true))
}
