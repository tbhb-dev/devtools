// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package contentposts finds skill scripts that bypass gh-post.
package contentposts

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Scripts lists the same recursive source boundary as the Python gate.
func Scripts(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(
		filepath.Join(root, "packages/agents/source"),
		func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && strings.HasSuffix(path, ".sh") {
				paths = append(paths, path)
			}
			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("list skill scripts: %w", err)
	}
	return paths, nil
}

// Scan reports file:line findings on stdout, errors on stderr, and returns
// 0 for clean input, 1 for findings, or 2 for any read or parse failure.
func Scan(paths []string, stdout, stderr io.Writer) int {
	status := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: read: %v\n", path, err)
			status = 2
			continue
		}
		lines, err := violations(data, path)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: parse: %v\n", path, err)
			status = 2
			continue
		}
		for _, line := range lines {
			_, _ = fmt.Fprintf(stdout, "%s:%d\n", path, line)
			if status == 0 {
				status = 1
			}
		}
	}
	return status
}

func violations(data []byte, name string) ([]uint, error) {
	tree, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(bytes.NewReader(data), name)
	if err != nil {
		return nil, fmt.Errorf("parse Bash: %w", err)
	}
	var lines []uint
	syntax.Walk(tree, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		words := make([]string, 0, len(call.Args))
		for _, arg := range call.Args {
			words = append(words, strings.Trim(string(data[arg.Pos().Offset():arg.End().Offset()]), "\"'"))
		}
		if postsContent(words) {
			lines = append(lines, call.Pos().Line())
		}
		return true
	})
	slices.Sort(lines)
	return slices.Compact(lines), nil
}

var endpointPattern = regexp.MustCompile(`/(comments|reviews)(/|$)`)

func postsContent(words []string) bool {
	for len(words) > 0 && (words[0] == "command" || words[0] == "env" || strings.Contains(words[0], "=")) {
		words = words[1:]
	}
	if len(words) < 3 || !slices.Contains([]string{"gh", "dt_gh"}, filepath.Base(words[0])) {
		return false
	}
	family, operation := words[1], words[2]
	if family == "pr" && operation == "edit" {
		return true
	}
	if slices.Contains([]string{"issue", "pr", "release"}, family) {
		if slices.Contains([]string{"create", "comment", "review"}, operation) {
			return true
		}
		return operation == "edit" && hasTextFlag(words[3:])
	}
	if family != "api" {
		return false
	}
	return postsAPI(words[2:])
}

func postsAPI(words []string) bool {
	endpoint, write, payload := false, false, false
	for _, word := range words {
		endpoint = endpoint || endpointPattern.MatchString(word)
		for _, method := range []string{"POST", "PATCH", "PUT", "DELETE"} {
			write = write || word == method || strings.TrimPrefix(word, "--method=") == method ||
				strings.TrimPrefix(word, "-X") == method
		}
		flag, _, _ := strings.Cut(word, "=")
		payload = payload || slices.Contains([]string{"-f", "-F", "--field", "--raw-field", "--input"}, flag)
	}
	return endpoint && (write || payload)
}

func hasTextFlag(words []string) bool {
	for _, word := range words {
		flag, _, _ := strings.Cut(word, "=")
		if slices.Contains(
			[]string{"-b", "-F", "--body", "--body-file", "--title", "--notes", "--notes-file", "$@"},
			flag,
		) ||
			strings.HasPrefix(word, "-b") ||
			strings.HasPrefix(word, "-F") {
			return true
		}
	}
	return false
}
