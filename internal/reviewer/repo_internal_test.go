// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package reviewer

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepository(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"pr", "review", "-R", "Owner/Repo"}, {"--repo=Owner/Repo"}, {"-ROwner/Repo"}, {"--repo", "https://github.com/Owner/Repo.git"}, {"-R=github.com/Owner/Repo"}} {
		repo, err := Repository(t.Context(), args)
		require.NoError(t, err)
		require.Equal(t, "owner/repo", repo)
	}
	for _, target := range []string{"git@github.com:Owner/Repo.git", "ssh://git@github.com/Owner/Repo.git", "https://github.com/Owner/Repo"} {
		repo, err := parseRepository(target)
		require.NoError(t, err)
		require.Equal(t, "owner/repo", repo)
	}
	for _, target := range []string{"other.com/owner/repo", "https://other.com/owner/repo", "owner", "../repo", "owner/..", "owner/repo/extra"} {
		_, err := parseRepository(target)
		require.Error(t, err)
	}
	for _, args := range [][]string{{"-R"}, {"-R", ""}, {"--repo="}} {
		_, err := Repository(t.Context(), args)
		require.Error(t, err)
	}
}

//nolint:paralleltest // Chdir changes the process working directory.
func TestOrigin(t *testing.T) {
	// Working directory changes are process-global.
	t.Chdir(t.TempDir())
	for _, args := range [][]string{{"init", "--bare"}, {"remote", "add", "origin", "git@github.com:Owner/Repo.git"}} {
		require.NoError(t, exec.CommandContext(t.Context(), "git", args...).Run())
	}
	repo, err := Repository(t.Context(), []string{"pr", "review"})
	require.NoError(t, err)
	require.Equal(t, "owner/repo", repo)
	repo, err = Repository(t.Context(), []string{"--", "-R", "other/repo"})
	require.NoError(t, err)
	require.Equal(t, "owner/repo", repo)
}
