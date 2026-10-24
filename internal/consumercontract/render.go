// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract

import (
	"fmt"
	"strings"
)

// Render writes the human-readable notes for a contract. The notes come from
// the structured source alone, so they can't drift from it. Every
// paragraph sits on one line, as the repository's Markdown rule requires.
func Render(doc *Document) string {
	var out strings.Builder
	out.WriteString(
		"<!-- Generated from " + ContractPath + " by `agentrepo contract render`. Do not edit by hand. -->\n\n",
	)
	out.WriteString("# Consumer transitions\n\n")
	out.WriteString(
		"This page lists what a consumer of this release keeps in `" + ConfFile + "` and what the consumer must change to adopt the release. The page in a release's tag describes that release, and the page on `main` describes the next one.\n\n",
	)
	out.WriteString("## Transitions\n\n")
	renderTransitions(&out, doc)
	out.WriteString("## Contract\n\n")
	fmt.Fprintf(
		&out,
		"Schema version %d. A required key makes the readers reject the file without it. A value of `nonempty` accepts any text, `empty` accepts a key with no value, and `exactly` accepts one string.\n\n",
		doc.SchemaVersion,
	)
	out.WriteString("| Setting | Required | Accepts |\n| --- | --- | --- |\n")
	for _, key := range doc.Keys {
		required := "no"
		if key.Required {
			required = "yes"
		}
		out.WriteString("| `" + key.Name + "` | " + required + " | " + acceptsText(key) + " |\n")
	}
	return out.String()
}

func acceptsText(key Key) string {
	parts := make([]string, 0, len(key.Accepts))
	for _, class := range key.Accepts {
		if class == ClassValue {
			parts = append(parts, "exactly `"+key.Value+"`")
			continue
		}
		parts = append(parts, class)
	}
	return strings.Join(parts, ", ")
}

func renderTransitions(out *strings.Builder, doc *Document) {
	if doc.Baseline {
		out.WriteString(
			"This release is the baseline for the contract. It doesn't change behavior, and its contract equals what the preceding release's readers accept.\n\n",
		)
	}
	if doc.Transition.None {
		out.WriteString("This release doesn't require a consumer change.\n\n")
		return
	}
	for index, entry := range doc.Transition.Entries {
		fmt.Fprintf(out, "### %d. %s: `%s`\n\n", index+1, kindTitle(entry.Kind), entry.Key)
		fmt.Fprintf(
			out,
			"Applies when %s in `%s`. Do this only once the consumer runs `%s` or later, because that is the first release whose reader accepts the target.\n\n",
			whenText(entry.AppliesWhen),
			entry.File,
			entry.AfterRelease,
		)
		if entry.TargetValue != "" {
			fmt.Fprintf(out, "Target value: `%s`.\n\n", entry.TargetValue)
		}
		fmt.Fprintf(out, "Verify: %s\n\n", entry.Verify)
		fmt.Fprintf(out, "Roll back: %s\n\n", entry.Rollback)
	}
}

func kindTitle(kind string) string {
	switch kind {
	case KindRequiredKeyAdded:
		return "Required key added"
	case KindRequiredValueChanged:
		return "Required value changed"
	default:
		return "Accepted value removed"
	}
}

func whenText(when string) string {
	if when == WhenKeyAbsent {
		return "the key is absent"
	}
	return "the key holds a value the new reader rejects"
}
