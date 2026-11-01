// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"fmt"
	"io"

	"github.com/tbhb-dev/devtools/internal/guard"
	"github.com/tbhb-dev/devtools/internal/hookio"
)

const exitBlocked = 2

type streams struct {
	in  io.Reader
	out io.Writer
	err io.Writer
}

// Run returns the hook exit status, and an allowed call writes to neither stream.
func Run(args []string, s streams) int {
	if len(args) != 1 {
		return usage(s.err)
	}
	skill := args[0]
	switch skill {
	case "pr", "merge-pr", "watch-pr", "fix-pr":
	default:
		return usage(s.err)
	}
	payload, err := hookio.DecodeStrict(s.in)
	if err != nil {
		return 0
	}
	reason := guard.Reason(skill, payload.ToolInput.Command)
	if reason == "" {
		return 0
	}
	if blockErr := hookio.Block(s.err, skill, reason); blockErr != nil {
		fmt.Fprintln(s.err, "guard-workflow:", blockErr)
	}
	return exitBlocked
}

func usage(w io.Writer) int {
	fmt.Fprintln(w, "usage: guard-workflow <pr|merge-pr|watch-pr|fix-pr>")
	return exitBlocked
}
