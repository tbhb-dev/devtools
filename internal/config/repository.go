// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package config

import (
	"errors"
	"strings"
)

var errRegularFile = errors.New("Repository configuration must be a regular file")

func repositoryKey(key string) bool {
	switch key {
	case "lint_commit_cmd", "required_hooks", "hook_marker", "review_max_rounds",
		"pr_template", "lint_pr_cmd", "pr_log_entry", "pr_related_pattern", "pr_title_check_cmd",
		"owner_paths", "mail_cmd", "mail_to", "mail_from", "lint_squash_cmd", "owner_review_label",
		"ruleset_dir", "golden_paths", "loosening_markers", "gate_config_paths", "disposable_ignored", "merge_ready_cmd":
		return true
	default:
		return false
	}
}

func validate(data string) error {
	seen := make(map[string]bool)
	var invalid []string
	for line := range strings.SplitSeq(data, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, ok := assignment(line)
		if !ok || seen[key] || !repositoryKey(key) {
			invalid = append(invalid, "Invalid or duplicate repository setting: "+key)
		}
		seen[key] = true
	}
	if len(invalid) != 0 {
		//nolint:err113 // Preserve each invalid key's diagnostic, in file order.
		return errors.New(strings.Join(invalid, "\n"))
	}
	return nil
}
