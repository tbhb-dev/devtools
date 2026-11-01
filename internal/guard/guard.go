// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package guard

import (
	"fmt"
	"regexp"
	"strings"
)

// Shell whitespace under the shells' LC_ALL=C includes vertical tab, which
// Go's [[:space:]] omits. These expressions intentionally skip parsing
// quoting or heredocs, and the flag expressions inspect the entire text.
const (
	space   = `[ \t\r\n\v\f]`
	atStart = `(^|[;&|(\n])` + space + `*`
)

// Reason returns an empty string to allow the command, or the frozen
// shell guard's denial reason. It ignores the tool name.
func Reason(skill, command string) string {
	switch skill {
	case "pr", "merge-pr":
		match := regexp.MustCompile(atStart + `gh` + space + `+pr` + space + `+([a-z-]+)`).FindStringSubmatch(command)
		if match == nil {
			return ""
		}
		// Only the first matching verb is judged, including in a compound call.
		verb := match[2]
		switch verb {
		case "list", "view", "diff", "status", "checks":
			return ""
		}
		if skill == "pr" {
			return fmt.Sprintf(publishReason, verb)
		}
		return fmt.Sprintf(mergeReason, verb)
	case "watch-pr":
		if watchingChecks(command) {
			return watchChecksReason
		}
		if call(command, `gh`+space+`+run`+space+`+watch`) {
			return watchRunReason
		}
	case "fix-pr":
		return fixReason(command)
	}
	return ""
}

func call(command, expression string) bool {
	return regexp.MustCompile(atStart + expression).MatchString(command)
}

func watchingChecks(command string) bool {
	return call(command, `gh`+space+`+pr`+space+`+checks`) &&
		(strings.Contains(command, "--watch") || strings.Contains(command, "--fail-fast"))
}

func fixReason(command string) string {
	if call(command, `gh`+space+`+run`+space+`+view`) &&
		regexp.MustCompile(`--log-failed|--log(`+space+`|$)`).MatchString(command) &&
		!strings.Contains(command, "--job") {
		return diagnoseReason
	}
	if call(command, `git`+space+`+push`) &&
		regexp.MustCompile(`--force(`+space+`|$)|`+space+`-f(`+space+`|$)`).MatchString(command) &&
		!strings.Contains(command, "--force-with-lease") {
		return forceReason
	}
	if watchingChecks(command) {
		return fixWatchReason
	}
	return ""
}
