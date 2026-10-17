#!/usr/bin/env bats

@test "scratch setup clears environment config and identities before committing" {
  source "${BATS_TEST_DIRNAME}/test_helper.bash"
  export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=commit.gpgSign GIT_CONFIG_VALUE_0=true
  export GIT_CONFIG_KEY_9=unused GIT_CONFIG_VALUE_9=unused
  export GIT_AUTHOR_NAME=Inherited GIT_AUTHOR_EMAIL=inherited@example.com
  export GIT_AUTHOR_DATE=invalid GIT_COMMITTER_DATE=invalid
  export GIT_COMMITTER_NAME=Inherited GIT_COMMITTER_EMAIL=inherited@example.com

  isolate_git

  [ -z "${GIT_CONFIG_COUNT+x}" ]
  [ -z "${GIT_CONFIG_KEY_0+x}${GIT_CONFIG_VALUE_0+x}" ]
  [ -z "${GIT_CONFIG_KEY_9+x}${GIT_CONFIG_VALUE_9+x}" ]
  [ -z "${GIT_AUTHOR_NAME+x}${GIT_AUTHOR_EMAIL+x}${GIT_AUTHOR_DATE+x}" ]
  [ -z "${GIT_COMMITTER_NAME+x}${GIT_COMMITTER_EMAIL+x}${GIT_COMMITTER_DATE+x}" ]
  git init -q --initial-branch=main "$BATS_TEST_TMPDIR/repo"
  cd "$BATS_TEST_TMPDIR/repo"
  git config user.name Test
  git config user.email test@example.com
  git commit -q --allow-empty -m fixture
  [ "$(git show -s --format='%an <%ae>|%cn <%ce>|%G?')" = 'Test <test@example.com>|Test <test@example.com>|N' ]
}
