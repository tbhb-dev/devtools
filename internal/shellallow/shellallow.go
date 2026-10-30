// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package shellallow finds the commands in a shell script that the
// repository's allow list doesn't permit.
//
// Scripts that call interpreters and data tools (Python, Ruby, sed, curl)
// break on hosts that lack them or carry a different version. The allow list
// holds the commands a script may run. Check walks the whole syntax tree, so
// a command inside a substitution, a pipeline stage, or a function body
// counts the same as one at the top level.
//
// The check goes by command name alone. An interpreter fed a heredoc or a
// `-c` or `-e` string is a finding on its own, wherever its input comes from,
// so the check never inspects the input. A banned tool that a wrapper such as
// `xargs` runs is missed.
package shellallow

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"mvdan.cc/sh/v3/syntax"
)

// Kind says why a command was reported.
type Kind string

const (
	// Banned marks a name that no allow list entry can permit.
	Banned Kind = "banned"
	// NotAllowed marks a literal name the allow list doesn't hold.
	NotAllowed Kind = "not on the allow list"
	// Unresolvable marks a command word the check can't read statically.
	Unresolvable Kind = "unresolvable"
)

// isBanned reports whether a command name is refused wherever it appears.
// The list includes the shell's own string-running builtin because the check
// can't read the string it runs.
func isBanned(name string) bool {
	switch name {
	case "ruby", "python", "python3", "perl", "node", "jq", "awk", "sed",
		"curl", "wget", "eval":
		return true
	}
	return false
}

// Finding is one reported command.
type Finding struct {
	File    string
	Line    uint
	Command string
	Kind    Kind
}

// String renders the finding as `file:line: command: reason`.
func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", f.File, f.Line, f.Command, f.Kind)
}

// Allowlist holds the command names that a script may run.
type Allowlist struct {
	allowed map[string]bool
}

// allowlistFile mirrors .devtools/shell-allowlist.toml. Each class has its
// own key for the reader's sake, and Load merges them.
type allowlistFile struct {
	Builtins  []string `toml:"builtins"`
	Tools     []string `toml:"tools"`
	Coreutils []string `toml:"coreutils"`
	Binaries  []string `toml:"binaries"`
}

// Load reads an allow list from file. An unknown key is an error, so a
// misspelled class can't silently allow nothing.
func Load(file string) (*Allowlist, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("reading: %w", err)
	}
	var parsed allowlistFile
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if decodeErr := dec.Decode(&parsed); decodeErr != nil {
		return nil, fmt.Errorf("parsing %s: %w", file, decodeErr)
	}
	list := &Allowlist{allowed: map[string]bool{}}
	for _, class := range [][]string{parsed.Builtins, parsed.Tools, parsed.Coreutils, parsed.Binaries} {
		for _, name := range class {
			list.allowed[name] = true
		}
	}
	return list, nil
}

// Check parses src as shell and returns each command it runs that the
// allow list doesn't permit, in source order. file only labels the findings.
func (a *Allowlist) Check(file string, src io.Reader) ([]Finding, error) {
	prog, err := syntax.NewParser().Parse(src, file)
	if err != nil {
		return nil, fmt.Errorf("parsing shell: %w", err)
	}

	c := checker{file: file, allowed: a.allowed}
	c.statements(prog, map[string]bool{})
	return c.findings, nil
}

type checker struct {
	file     string
	allowed  map[string]bool
	findings []Finding
}

// Only unconditional declarations establish bindings for later statements.
// Branches, loops and calls that can change bindings discard that knowledge.
func (c *checker) statements(root syntax.Node, defined map[string]bool) {
	for _, node := range children(root) {
		stmt, ok := node.(*syntax.Stmt)
		if !ok {
			continue
		}
		switch cmd := stmt.Cmd.(type) {
		case *syntax.FuncDecl:
			// The body runs later, with bindings the check can't know here.
			c.walk(stmt, map[string]bool{})
			if !stmt.Background {
				defined[cmd.Name.Value] = true
			}
		case *syntax.CallExpr:
			c.walk(stmt, defined)
		default:
			c.walk(stmt, map[string]bool{})
			clear(defined)
		}
	}
}

func (c *checker) walk(root syntax.Node, defined map[string]bool) {
	syntax.Walk(root, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.FuncDecl:
			c.walk(n.Body, map[string]bool{})
			return false
		case *syntax.Block, *syntax.Subshell, *syntax.CmdSubst, *syntax.ProcSubst:
			c.statements(n, maps.Clone(defined))
			return false
		case *syntax.CallExpr:
			c.call(n, defined)
		}
		return true
	})
}

// children returns immediate children in source order.
func children(root syntax.Node) []syntax.Node {
	var nodes []syntax.Node
	syntax.Walk(root, func(node syntax.Node) bool {
		if node == root {
			return true
		}
		if node != nil {
			nodes = append(nodes, node)
		}
		return false
	})
	return nodes
}

func (c *checker) call(call *syntax.CallExpr, defined map[string]bool) {
	if len(call.Args) == 0 {
		return // A bare assignment doesn't run a command.
	}
	word := call.Args[0]
	name, literal := literalWord(word)
	var kind Kind
	switch {
	case !literal:
		var text strings.Builder
		// Printing to a Builder can't fail.
		_ = syntax.NewPrinter().Print(&text, word)
		name, kind = text.String(), Unresolvable
	case isBanned(path.Base(name)):
		kind = Banned
	case !c.allowed[name] && !defined[name]:
		kind = NotAllowed
	}
	if kind != "" {
		c.findings = append(c.findings, Finding{c.file, word.Pos().Line(), name, kind})
	}
	if !literal || defined[name] {
		clear(defined)
	}
	switch name {
	case "unset", "source", ".", "eval", "builtin", "command", "trap":
		clear(defined)
	}
}

// literalWord returns a word's text when nothing in it expands: plain text
// and quoted plain text, joined.
func literalWord(word *syntax.Word) (string, bool) {
	if word.Lit() == "[" {
		return "[", true // The test builtin has no closing glob bracket.
	}
	var text strings.Builder
	for _, part := range word.Parts {
		if !appendLiteral(&text, part, false) {
			return "", false
		}
	}
	return text.String(), true
}

func appendLiteral(text *strings.Builder, part syntax.WordPart, quoted bool) bool {
	switch p := part.(type) {
	case *syntax.Lit:
		return appendText(text, p.Value, quoted)
	case *syntax.SglQuoted:
		if p.Dollar {
			return appendANSI(text, p.Value)
		}
		text.WriteString(p.Value)
	case *syntax.DblQuoted:
		if p.Dollar {
			return false // Locale translation can change the executable.
		}
		for _, inner := range p.Parts {
			if !appendLiteral(text, inner, true) {
				return false
			}
		}
	default:
		return false
	}
	return true
}

func appendText(text *strings.Builder, value string, quoted bool) bool {
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch == '\\' && i+1 < len(value) &&
			(!quoted || strings.ContainsRune("$`\"\\\n", rune(value[i+1]))) {
			i++
			if value[i] != '\n' {
				text.WriteByte(value[i])
			}
			continue
		}
		if !quoted && strings.ContainsRune("*?[{~", rune(ch)) {
			return false
		}
		text.WriteByte(ch)
	}
	return true
}

// Decode escapes shared with Go in dollar-prefixed single quotes.
// Other forms fail closed, including a null byte, which shells can
// use to truncate the quoted value.
func appendANSI(text *strings.Builder, value string) bool {
	for len(value) > 0 {
		if value[0] != '\\' {
			text.WriteByte(value[0])
			value = value[1:]
			continue
		}
		ch, wide, tail, err := strconv.UnquoteChar(value, '\'')
		if err != nil || ch == 0 {
			return false
		}
		if !wide && ch >= 0 && ch <= 255 {
			text.WriteByte(byte(ch))
		} else {
			text.WriteRune(ch)
		}
		value = tail
	}
	return true
}
