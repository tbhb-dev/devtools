// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"unicode"
)

const transcriptWindow = 65536

type tokenCounts struct {
	In        json.RawMessage `json:"in,omitempty"`
	Out       json.RawMessage `json:"out,omitempty"`
	CacheRead json.RawMessage `json:"cache_read,omitempty"`
	Error     string          `json:"error,omitempty"`
}

func transcriptTokens(name *string) *tokenCounts {
	failure := func(kind string) *tokenCounts { return &tokenCounts{Error: kind} }
	if name == nil {
		return failure("missing")
	}
	file, err := os.Open(*name)
	if err != nil {
		return failure("missing")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return failure("missing")
	}
	offset := max(int64(0), info.Size()-transcriptWindow)
	data := make([]byte, info.Size()-offset)
	n, err := file.ReadAt(data, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return failure("missing")
	}
	data = data[:n]
	if offset > 0 {
		newline := bytes.IndexByte(data, '\n')
		if newline < 0 {
			return failure("window")
		}
		data = data[newline+1:]
	}
	// A complete JSON line ends with a newline.
	lines := bytes.Split(data, []byte{'\n'})
	lines = lines[:len(lines)-1]
	for _, line := range slices.Backward(lines) {
		var response struct {
			Type      string          `json:"type"`
			In        json.RawMessage `json:"input_tokens"`
			Out       json.RawMessage `json:"output_tokens"`
			CacheRead json.RawMessage `json:"cache_read_tokens"`
		}
		// Read the type separately so invalid counts still produce a format error.
		var header struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &header) != nil || header.Type != "PLANNER_RESPONSE" {
			continue
		}
		if json.Unmarshal(line, &response) != nil || !validCount(response.In) || !validCount(response.Out) ||
			!validCount(response.CacheRead) {
			return failure("format")
		}
		return &tokenCounts{In: response.In, Out: response.Out, CacheRead: response.CacheRead}
	}
	return failure("window")
}

// simpleCommands recognizes separators only outside quotes and keeps quoted words intact.
func simpleCommands(line string) ([][]string, bool) {
	var commands [][]string
	var words []string
	var word strings.Builder
	var quote rune
	started, escaped := false, false
	finishWord := func() {
		if started {
			words = append(words, word.String())
			word.Reset()
			started = false
		}
	}
	finishCommand := func() {
		finishWord()
		if len(words) > 0 {
			commands = append(commands, words)
			words = nil
		}
	}
	for index, char := range line {
		switch {
		case escaped:
			word.WriteRune(char)
			started = true
			escaped = false
		case char == '\\' && quote != '\'':
			escaped = true
			started = true
		case quote != 0:
			if char == quote {
				quote = 0
			} else {
				word.WriteRune(char)
			}
		case strings.ContainsRune("'\"", char):
			quote = char
			started = true
		case commandSeparator(line, index, char):
			finishCommand()
		case unicode.IsSpace(char):
			finishWord()
		default:
			word.WriteRune(char)
			started = true
		}
	}
	if quote != 0 || escaped {
		return nil, false
	}
	finishCommand()
	return commands, true
}

func commandAttempts(line string) []string {
	commands, ok := simpleCommands(line)
	if !ok {
		return nil
	}
	var attempts []string
	for _, words := range commands {
		if slices.Contains(words, "--dry-run") {
			continue
		}
		if gitCommit(words) {
			attempts = append(attempts, "commit")
		}
		if len(words) >= 3 && (words[0] == "gh" || words[0] == "gh-post") && words[1] == "pr" && words[2] == "create" {
			attempts = append(attempts, "pr_publish")
		}
		if marker := scriptAttempt(words); marker != "" {
			attempts = append(attempts, marker)
		}
	}
	slices.Sort(attempts)
	return slices.Compact(attempts)
}

func gitCommit(words []string) bool {
	if words[0] != "git" {
		return false
	}
	for i := 1; i < len(words); i++ {
		if words[i] == "commit" {
			return true
		}
		if (words[i] != "-C" && words[i] != "-c") || i+1 >= len(words) {
			return false
		}
		if words[i] == "-c" && !strings.Contains(words[i+1], "=") {
			return false
		}
		i++
	}
	return false
}

func commandSeparator(line string, index int, char rune) bool {
	if char == ';' || char == '|' || char == '\n' {
		return true
	}
	if char != '&' {
		return false
	}
	return (index+1 < len(line) && line[index+1] == '&') || (index > 0 && line[index-1] == '&')
}

func scriptAttempt(words []string) string {
	script := words[0]
	if (script == "bash" || script == "sh") && len(words) > 1 {
		script = words[1]
	}
	switch path.Base(script) {
	case "commit.sh":
		return "commit"
	case "create-pr.sh", "publish.sh":
		if !slices.Contains(words, "--check-description") {
			return "pr_publish"
		}
	}
	return ""
}

func validCount(value json.RawMessage) bool {
	if len(value) == 0 {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
