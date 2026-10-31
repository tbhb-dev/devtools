// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// Run checks the given paths, or the scripts changed since a base ref when the arguments start with the changed-files flag.
func Run(args []string, out io.Writer) int {
	if len(args) == 1 && args[0] == "--summary" {
		return summary(out)
	}
	if len(args) == 2 && args[0] == "--changed" {
		paths, err := changedFiles(args[1])
		if err != nil {
			fmt.Fprintln(out, err)
			return 1
		}
		args = paths
	}
	allowed, err := readAllowlist()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	status := 0
	for _, path := range args {
		if !checkFile(path, allowed, out) {
			status = 1
		}
	}
	return status
}

func checkFile(path string, allowed map[string]bool, out io.Writer) bool {
	violations, failed := scanFile(path, allowed, out)
	return violations == 0 && !failed
}

func scanFile(path string, allowed map[string]bool, out io.Writer) (int, bool) {
	//nolint:gosec // G703: reading the caller's exact file paths is this command's purpose.
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(out, err)
		return 0, true
	}
	tree, err := syntax.NewParser().Parse(strings.NewReader(string(data)), path)
	if err != nil {
		// Parser errors can quote source text. Keep command arguments out of diagnostics.
		fmt.Fprintf(out, "%s: invalid shell syntax\n", path)
		return 0, true
	}
	violations := 0
	catHeredocs := make(map[*syntax.Redirect]bool)
	syntax.Walk(tree, func(node syntax.Node) bool {
		switch node := node.(type) {
		case *syntax.Stmt:
			// Walk reaches a statement before its redirects, so this marks the heredocs fed to cat.
			if runsCat(node) {
				for _, redirect := range node.Redirs {
					catHeredocs[redirect] = true
				}
			}
		case *syntax.Redirect:
			// A cat heredoc holds data, including substitutions. Other heredocs run theirs.
			return node.Hdoc == nil || !catHeredocs[node]
		case *syntax.CallExpr:
			if !checkCall(path, node, allowed, out) {
				violations++
			}
		}
		return true
	})
	return violations, false
}

func checkCall(path string, call *syntax.CallExpr, allowed map[string]bool, out io.Writer) bool {
	if len(call.Args) == 0 {
		return true
	}
	word := call.Args[0]
	name, literal := commandName(word)
	if !literal {
		fmt.Fprintf(out, "%s:%d: unresolvable\n", path, word.Pos().Line())
		return false
	}
	if !allowed[name] {
		fmt.Fprintf(out, "%s:%d: %s: not allowed\n", path, word.Pos().Line(), name)
		return false
	}
	return true
}

func runsCat(stmt *syntax.Stmt) bool {
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	name, literal := commandName(call.Args[0])
	return literal && name == "cat"
}

func commandName(word *syntax.Word) (string, bool) {
	literal := true
	syntax.Walk(word, func(node syntax.Node) bool {
		switch node.(type) {
		case *syntax.Word, *syntax.Lit, *syntax.SglQuoted, *syntax.DblQuoted:
			return true
		default:
			if node != nil {
				literal = false
			}
			return false
		}
	})
	if !literal {
		return "", false
	}
	// Fields removes shell quoting. A nil config disables wildcard expansion.
	names, err := expand.Fields(nil, word)
	if err != nil || len(names) != 1 {
		return "", false
	}
	return names[0], true
}
