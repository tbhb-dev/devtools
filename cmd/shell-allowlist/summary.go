// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/tbhb-dev/devtools/internal/shell"
)

// summary is informational: failures are reported without failing the future burn-down job.
func summary(out io.Writer) int {
	fmt.Fprint(out, "## Shell burn-down\n\n")
	files, err := gitOutput("ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		fmt.Fprintln(out, err)
		return 0
	}
	allowed, err := readAllowlist()
	if err != nil {
		fmt.Fprintln(out, err)
		return 0
	}
	paths := scriptPaths(files)
	slices.Sort(paths)
	paths = slices.Compact(paths)
	var rows strings.Builder
	total, errors := 0, 0
	escape := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "|", "&#124;", "\n", "&#10;", "\r", "&#13;")
	for _, path := range paths {
		count, failed := scanFile(path, allowed, io.Discard)
		analysisErrors := 0
		if failed {
			analysisErrors = 1
		}
		total += count
		errors += analysisErrors
		fmt.Fprintf(&rows, "| %s | %d | %d |\n", escape.Replace(path), count, analysisErrors)
	}
	fmt.Fprintf(out, "Total: %d violations across %d scripts; %d analysis errors.\n\n", total, len(paths), errors)
	fmt.Fprintln(out, "| Script | Violations | Analysis errors |\n| --- | ---: | ---: |")
	fmt.Fprint(out, rows.String())
	return 0
}

func readAllowlist() (*shell.Allowlist, error) {
	allowed, err := shell.Load(".devtools/shell-allowlist.toml")
	if err != nil {
		return nil, fmt.Errorf("read allow list: %w", err)
	}
	return allowed, nil
}
