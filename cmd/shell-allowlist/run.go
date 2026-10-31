// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// Run checks only the given paths, using the allow list in the current directory.
func Run(args []string, out io.Writer) int {
	data, err := os.ReadFile(".devtools/shell-allowlist.toml")
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	var config struct {
		Commands []string `toml:"commands"`
	}
	if err = toml.Unmarshal(data, &config); err != nil {
		fmt.Fprintln(out, ".devtools/shell-allowlist.toml: invalid allow list")
		return 1
	}
	allowed := make(map[string]bool, len(config.Commands))
	for _, command := range config.Commands {
		allowed[command] = true
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
	//nolint:gosec // G703: reading the caller's exact file paths is this command's purpose.
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(out, err)
		return false
	}
	tree, err := syntax.NewParser().Parse(strings.NewReader(string(data)), path)
	if err != nil {
		// Parser errors can quote source text. Keep command arguments out of diagnostics.
		fmt.Fprintf(out, "%s: invalid shell syntax\n", path)
		return false
	}
	clean := true
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
				clean = false
			}
		}
		return true
	})
	return clean
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
