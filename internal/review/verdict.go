// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package review

import (
	"bytes"
	"fmt"
	"io"
)

// Verdict classifies a complete reviewer reply using byte-level normalization.
// A passing verdict must be the only substantive line. Findings may follow
// a request for changes.
func Verdict(input io.Reader) (string, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return "", fmt.Errorf("read verdict: %w", err)
	}
	for _, b := range data {
		if b < 32 && b != '\t' && b != '\n' && b != '\r' {
			return "unreadable", nil
		}
	}
	var first []byte
	count := 0
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		line = bytes.TrimRight(line, " \t")
		if len(line) == 0 {
			continue
		}
		if count == 0 {
			first = line
		}
		count++
	}
	switch {
	case count == 1 && bytes.Equal(first, []byte("VERDICT: PASS")):
		return "pass", nil
	case count >= 1 && bytes.Equal(first, []byte("VERDICT: CHANGES REQUIRED")):
		return "changes", nil
	default:
		return "unreadable", nil
	}
}
