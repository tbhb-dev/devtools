// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Offline counting backends and a discovery failure injector for shell parity.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	mode, stage := os.Getenv("TOKEN_FAIL"), os.Getenv("TOKEN_FAIL_AT")
	if name == "agentrepo" {
		fmt.Fprintln(os.Stderr, "PATH agentrepo invoked")
		os.Exit(99)
	}
	if name == "git" {
		if strings.Contains(strings.Join(os.Args[1:], " "), "ls-files") {
			if stage == "discovery" {
				fail(mode)
				return
			}
			if stage == "measurement" {
				fmt.Println("missing-file")
				return
			}
		}
		cmd := exec.Command(os.Getenv("TOKEN_REAL_GIT"), os.Args[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			os.Exit(23)
		}
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}
	text := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	if path := os.Getenv("TOKEN_LOG"); path != "" {
		f, openErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if openErr != nil {
			panic(openErr)
		}
		fmt.Fprintf(f, "%s %s %s %s %s %q\n", name, os.Getenv("CLAUDE_MODEL"), os.Getenv("CODEX_MODEL"), os.Getenv("CODEX_ENCODING"), os.Args[len(os.Args)-1], text)
		f.Close()
	}
	if stage == "count" || (stage == "claude" && !strings.Contains(text, "content: |\n      x\n")) || (stage == "bundled" && strings.Contains(text, "bundled content")) {
		fail(mode)
		return
	}
	if stage == "second" && strings.Contains(text, "body") {
		if strings.Contains(text, "second") {
			fail(mode)
			return
		}
	}
	if stage == "total" {
		fmt.Println("9223372036854775807")
		return
	}
	if name == "ant" {
		payload := strings.SplitAfterN(text, "    content: |\n", 2)[1]
		payload = strings.TrimSuffix(payload, "\n")
		lines := strings.Split(payload, "\n")
		for i := range lines {
			lines[i] = strings.TrimPrefix(lines[i], "      ")
		}
		fmt.Printf("{\"input_tokens\":%d}\n", 7+len(strings.Join(lines, "\n")))
	} else {
		fmt.Println(len(text))
	}
}

func fail(mode string) {
	switch mode {
	case "partial":
		if filepath.Base(os.Args[0]) == "ant" {
			fmt.Println(`{"input_tokens":10}`)
		} else {
			fmt.Println(10)
		}
		os.Exit(23)
	case "invalid":
		fmt.Println("invalid")
	default:
		os.Exit(23)
	}
}
