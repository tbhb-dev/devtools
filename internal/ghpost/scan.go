// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"regexp"
	"strings"
)

// Finding never includes the matching text.
type Finding struct {
	Category string
	Line     int
}

// Scan checks one text field. Line numbers are local to that field.
func Scan(text string, private []string, secretsOnly bool) []Finding {
	patterns := []struct{ category, pattern string }{
		{
			"secret",
			`(gh[pours]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|-----BEGIN ([A-Z0-9]+ )*PRIVATE KEY( BLOCK)?-----|op:` + `//)`,
		},
	}
	if !secretsOnly {
		patterns = append(
			patterns,
			struct{ category, pattern string }{"file-url", `(?i)file://`},
			struct{ category, pattern string }{"home-path", `/(Users|home)/[A-Za-z_][A-Za-z0-9_.-]*/`},
		)
	}
	var findings []Finding
	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		for i, line := range strings.Split(text, "\n") {
			if re.MatchString(line) {
				findings = append(findings, Finding{p.category, i + 1})
			}
		}
	}
	if !secretsOnly {
		findings = append(findings, privateFindings(text, private)...)
	}
	return findings
}

func privateFindings(text string, private []string) []Finding {
	var findings []Finding
	for i, line := range strings.Split(text, "\n") {
		for _, literal := range private {
			if literal != "" && strings.Contains(strings.ToLower(line), strings.ToLower(literal)) {
				findings = append(findings, Finding{"private-string", i + 1})
				break
			}
		}
	}
	return findings
}
