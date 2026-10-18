// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package commitlint parses and checks commit messages.
package commitlint

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	rulesAll     = "all"
	contentStart = 2 // Subject and its separating blank line precede content.
)

var (
	scissors            = regexp.MustCompile(`^[#;] *-+ *>8 *-+ *$`)
	headerPattern       = regexp.MustCompile(`^([a-z]+)(\([^)]+\))?(!)?: (.+)$`)
	trailerPattern      = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9-]*|BREAKING[ -]CHANGE): .+$`)
	continuationPattern = regexp.MustCompile(`^[[:space:]]+[^[:space:]]`)
	signoffPattern      = regexp.MustCompile(`^Signed-off-by: .+$`)
	assistedPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*:[a-z0-9][a-z0-9.-]*( [a-z0-9][a-z0-9.-]*)*$`)
	llmPattern          = regexp.MustCompile(`(?i)\b(claude|chatgpt|copilot|gpt|gemini|bard|anthropic|openai|ai|llm)\b`)
)

// Message keeps raw lines for trailer compatibility and cleaned lines for shape checks.
type Message struct {
	Raw         []string
	Lines       []string
	FooterStart int
}

// Parse removes git scaffolding and classifies the final trailer block.
func Parse(raw string) Message {
	m := Message{Raw: strings.Split(strings.TrimRight(raw, "\n"), "\n"), FooterStart: -1}
	pending := false
	for _, line := range m.Raw {
		if scissors.MatchString(line) {
			break
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimRightFunc(line, unicode.IsSpace)
		if line == "" {
			pending = len(m.Lines) > 0
			continue
		}
		if pending {
			m.Lines = append(m.Lines, "")
			pending = false
		}
		m.Lines = append(m.Lines, line)
	}
	if len(m.Lines) == 0 {
		m.Lines = []string{""}
	}
	m.FooterStart = footerStart(m.Lines)
	return m
}

func footerStart(lines []string) int {
	if len(lines) <= contentStart {
		return -1
	}
	j := len(lines) - 1
	for j >= contentStart && (trailerPattern.MatchString(lines[j]) || continuationPattern.MatchString(lines[j])) {
		j--
	}
	if j < contentStart {
		return contentStart
	}
	if lines[j] == "" {
		return j + 1
	}
	return -1
}

// Check runs shape, trailers, or all rules, retaining the legacy diagnostic prefixes.
func Check(m Message, c Config, rules string) []string {
	var errors []string
	if rules == "shape" || rules == rulesAll {
		errors = append(errors, checkShape(m, c)...)
	}
	if rules == "trailers" || rules == rulesAll {
		errors = append(errors, checkTrailers(m, c)...)
	}
	return errors
}

func checkShape(m Message, c Config) []string {
	var errors []string
	report := func(format string, args ...any) { errors = append(errors, "commitlint: "+fmt.Sprintf(format, args...)) }
	header := m.Lines[0]
	length := utf8.RuneCountInString(header)
	if length < c.HeaderMinLength {
		report("subject is too short (%d < %d): \"%s\"", length, c.HeaderMinLength, header)
	}
	if length > c.HeaderMaxLength {
		report("subject is too long (%d > %d): \"%s\"", length, c.HeaderMaxLength, header)
	}
	match := headerPattern.FindStringSubmatch(header)
	if match == nil {
		report("subject must follow Conventional Commits: <type>(<scope>)?(!)?: <description>")
	} else if !slices.Contains(c.Types, match[1]) {
		report("unknown type \"%s\"; allowed: %s", match[1], strings.Join(c.Types, ", "))
	}
	if len(m.Lines) > 1 && m.Lines[1] != "" {
		report("the subject must be separated from the body by a blank line")
	}
	errors = append(errors, checkLineLengths(m, c)...)
	if c.RequireSignedOffBy && !slices.ContainsFunc(m.Lines, signoffPattern.MatchString) {
		report("missing required Signed-off-by trailer (DCO)")
	}
	return errors
}

func checkLineLengths(m Message, c Config) []string {
	var findings []string
	for i := contentStart; i < len(m.Lines); i++ {
		region, limit := "body", c.BodyMaxLineLength
		if m.FooterStart >= 0 && i >= m.FooterStart {
			region, limit = "footer", c.FooterMaxLineLength
		}
		if length := utf8.RuneCountInString(m.Lines[i]); length > limit {
			findings = append(
				findings,
				fmt.Sprintf("commitlint: %s line %d exceeds %d chars (%d)", region, i+1, limit, length),
			)
		}
	}
	return findings
}

func checkTrailers(m Message, c Config) []string {
	var errors []string
	report := func(format string, args ...any) {
		errors = append(errors, "commit-trailers: "+fmt.Sprintf(format, args...))
	}
	assisted := slices.IndexFunc(m.Raw, func(line string) bool { return strings.HasPrefix(line, "Assisted-by:") }) + 1
	signed := slices.IndexFunc(m.Raw, func(line string) bool { return strings.HasPrefix(line, "Signed-off-by:") }) + 1
	for i, line := range m.Raw {
		switch {
		case strings.HasPrefix(line, "Assisted-by:"):
			value := strings.TrimSpace(strings.TrimPrefix(line, "Assisted-by:"))
			if !assistedPattern.MatchString(value) {
				report(
					"malformed Assisted-by value: expected AGENT_NAME:MODEL_VERSION [TOOL...] (line %d: \"%s\")",
					i+1,
					value,
				)
			}
		case strings.HasPrefix(line, "Co-authored-by:"):
			value := strings.TrimSpace(strings.TrimPrefix(line, "Co-authored-by:"))
			if llmPattern.MatchString(value) {
				report(
					"forbidden Co-authored-by attribution to an LLM; use Assisted-by instead (line %d: \"%s\")",
					i+1,
					value,
				)
			}
		}
	}
	if assisted > 0 && signed > 0 && assisted > signed {
		report("trailer order: Assisted-by must appear before Signed-off-by (line %d after line %d)", assisted, signed)
	}
	if c.RequireSignedOffBy && signed == 0 {
		report("missing required Signed-off-by trailer (DCO)")
	}
	return errors
}
