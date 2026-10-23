// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package review

import (
	"bytes"
	"fmt"
	"io"
)

// Verdict classifies a complete reviewer reply using the strict verdict format.
func Verdict(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read reviewer reply: %w", err)
	}
	for _, b := range data {
		if b < ' ' && b != '\t' && b != '\n' && b != '\r' {
			return "unreadable", nil
		}
	}
	var first []byte
	n := 0
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		// Match the shell: one CR, then trailing spaces and tabs.
		line = bytes.TrimSuffix(line, []byte{'\r'})
		line = bytes.TrimRight(line, " \t")
		if len(line) == 0 {
			continue
		}
		n++
		if n == 1 {
			first = line
		}
	}
	switch {
	case n == 1 && string(first) == "VERDICT: PASS":
		return "pass", nil
	case string(first) == "VERDICT: CHANGES REQUIRED":
		return "changes", nil
	default:
		return "unreadable", nil
	}
}
