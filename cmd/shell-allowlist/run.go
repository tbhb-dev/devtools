// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tbhb-dev/devtools/internal/shell"
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

func checkFile(path string, allowed *shell.Allowlist, out io.Writer) bool {
	violations, failed := scanFile(path, allowed, out)
	return violations == 0 && !failed
}

func scanFile(path string, allowed *shell.Allowlist, out io.Writer) (int, bool) {
	//nolint:gosec // G703: reading the caller's exact file paths is this command's purpose.
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(out, err)
		return 0, true
	}
	findings, err := allowed.Check(path, strings.NewReader(string(data)))
	if err != nil {
		// Parser errors can quote source text. Keep command arguments out of diagnostics.
		fmt.Fprintf(out, "%s: invalid shell syntax\n", path)
		return 0, true
	}
	for _, finding := range findings {
		fmt.Fprintln(out, finding.String())
	}
	return len(findings), false
}
