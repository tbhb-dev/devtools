#!/usr/bin/env bats
setup() {
  source "${BATS_TEST_DIRNAME}/../merge-pr/owner-label.bash"
  owner_label_setup agent
}

@test "current owner approval releases the label and reports login and head" {
  owner_label_release
}

@test "old head owner approval keeps the label hold" {
  owner_label_old_head
}

@test "later owner changes requested keeps the label hold" {
  owner_label_later_review CHANGES_REQUESTED
}

@test "dismissed owner approval keeps the label hold" {
  owner_label_later_review DISMISSED
}

@test "no configured merge owner keeps the label hold" {
  owner_label_no_owner
}

@test "required reviewer alone cannot release the label" {
  owner_label_other_reviewer required
}

@test "another login cannot release the label" {
  owner_label_other_reviewer other
}

@test "owner label release still requires the required reviewer" {
  owner_label_missing_reviewer
}

@test "owner label release still requires passing checks" {
  owner_label_failed_check
}

@test "head moved after review holds at the final check" {
  owner_label_final_drift head
}

@test "owner changes requested on the same head holds at the final check" {
  owner_label_final_drift review
}
