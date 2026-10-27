// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package hygiene

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoveryFailure(t *testing.T) {
	t.Parallel()
	for _, failed := range []int{1, 2} {
		t.Run(strconv.Itoa(failed), func(t *testing.T) {
			t.Parallel()
			calls := 0
			run := func(_ context.Context, _, name string, _ ...string) ([]byte, []byte, int) {
				require.Equal(t, "git", name)
				calls++
				if calls == failed {
					return []byte("tools/partial.sh\n"), []byte("discovery broke\n"), 23
				}
				return nil, nil, 0
			}
			_, status := discover(t.Context(), t.TempDir(), run, &bytes.Buffer{})
			require.Equal(t, 23, status)
			require.Equal(t, failed, calls)
		})
	}
}

func TestDiagnosticFailures(t *testing.T) {
	t.Parallel()
	for _, input := range []string{`{`, `{"file":"x","range":{"start":{"line":"bad"}}}`, `{"file":"x","range":{"start":{"line":0}},"ruleId":"r","message":"m"} garbage`} {
		_, err := diagnostics([]byte(input))
		require.Error(t, err)
	}
	for _, status := range []int{0, 1, 2, 23, 127} {
		var out, stderr bytes.Buffer
		run := func(_ context.Context, _, _ string, _ ...string) ([]byte, []byte, int) {
			return []byte(
					`{"file":"x","range":{"start":{"line":0}},"ruleId":"r","message":"m"}`,
				), []byte(
					"checker diagnostic\n",
				), status
		}
		count, code := structural(t.Context(), "", []string{"x"}, run, &out, &stderr)
		if status <= 1 {
			require.Equal(t, 1, count)
			require.Zero(t, code)
			require.Empty(t, stderr.String())
		} else {
			require.Zero(t, count)
			require.Equal(t, 1, code)
			require.Contains(t, stderr.String(), "checker diagnostic")
			require.Empty(t, out.String())
		}
	}
}

func TestTextPredicates(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		text  string
		rules []string
	}{
		{"# git show\n# gh pr\n", nil},
		{"echo 'git show'\n", []string{"missing-locale-pin"}},
		{"my-git show\n", nil},
		{"git status\nexport LC_ALL=anything\n", nil},
		{"gh pr view\n", []string{"missing-locale-pin", "missing-gh-unset"}},
		{"gh pr view\nexport LC_ALL=C\nunset GH_REPO\n", nil},
		{"gh pr view\nexport LC_ALL=C\n unset GH_REPO GH_HOST\n", []string{"missing-gh-unset"}},
	} {
		var out bytes.Buffer
		count := textual("x", []byte(tc.text), &out)
		require.Equal(t, len(tc.rules), count)
		for _, rule := range tc.rules {
			require.Contains(t, out.String(), rule)
		}
	}
}

func TestExplicitSelection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "ordinary"), []byte("echo fine\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "exempt"), []byte("# hygiene-exempt: reason\ngh pr\n"), 0o600))
	paths, count, err := selectFiles(root, []string{"missing", "ordinary", "exempt"}, &bytes.Buffer{})
	require.NoError(t, err)
	require.Equal(t, []string{"ordinary"}, paths)
	require.Zero(t, count)
}

func TestLiteralDiagnosticSorting(t *testing.T) {
	t.Parallel()
	input := `{"file":"z","range":{"start":{"line":0}},"ruleId":"r","message":"back\\slash"}
{"file":"a","range":{"start":{"line":9}},"ruleId":"r","message":"z"}
{"file":"a","range":{"start":{"line":1}},"ruleId":"r","message":"b"}
{"file":"a","range":{"start":{"line":1}},"ruleId":"r","message":"a"}`
	got, err := diagnostics([]byte(input))
	require.NoError(t, err)
	require.Equal(t, []string{"a\t2\tr\ta", "a\t2\tr\tb", "a\t10\tr\tz", `z` + "\t1\tr\t" + `back\slash`}, got)
}
