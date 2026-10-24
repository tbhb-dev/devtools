<!-- Generated from .devtools/consumer-contract.toml by `agentrepo contract render`. Do not edit by hand. -->

# Consumer transitions

This page lists what a consumer of this release keeps in `.agents/agent-skills.conf` and what the consumer must change to adopt the release. The page in a release's tag describes that release, and the page on `main` describes the next one.

## Transitions

This release is the baseline for the contract. It doesn't change behavior, and its contract equals what the preceding release's readers accept.

This release doesn't require a consumer change.

## Contract

Schema version 1. A required key makes the readers reject the file without it. A value of `nonempty` accepts any text, `empty` accepts a key with no value, and `exactly` accepts one string.

| Setting | Required | Accepts |
| --- | --- | --- |
| `lint_commit_cmd` | no | empty, nonempty |
| `required_hooks` | no | empty, nonempty |
| `hook_marker` | no | empty, nonempty |
| `review_max_rounds` | no | empty, nonempty |
| `pr_template` | no | empty, nonempty |
| `lint_pr_cmd` | no | empty, nonempty |
| `pr_log_entry` | no | empty, nonempty |
| `pr_related_pattern` | no | empty, nonempty |
| `pr_title_check_cmd` | no | empty, nonempty |
| `mail_cmd` | no | empty, nonempty |
| `mail_to` | no | empty, nonempty |
| `mail_from` | no | empty, nonempty |
| `lint_squash_cmd` | no | empty, nonempty |
| `owner_review_label` | no | empty, nonempty |
| `ruleset_dir` | no | empty, nonempty |
| `merge_ready_cmd` | no | empty, nonempty |
| `constitutional_paths` | no | nonempty |
| `owner_paths` | yes | nonempty |
| `golden_paths` | yes | nonempty |
| `loosening_markers` | yes | nonempty |
| `gate_config_paths` | yes | nonempty |
| `disposable_ignored` | yes | nonempty |
