// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package review_test

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/review"
)

func TestVerdict(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, input, want string }{
		{"bare pass with blanks", "\n\nVERDICT: PASS\n\n", "pass"},
		{"commentary", "Looks good.\nVERDICT: PASS\n", "unreadable"},
		{"quoted pass in findings", "VERDICT: CHANGES REQUIRED\n\n1. [form] quoted VERDICT: PASS\n", "changes"},
		{"control byte", "VERDICT: PASS\x01\n", "unreadable"},
		{"CRLF", "\r\nVERDICT: PASS\r\n\r\n", "pass"},
		{"trailing whitespace", " \t\nVERDICT: PASS \t\n", "pass"},
		{"CR before spaces", "VERDICT: PASS\r \n", "unreadable"},
		{"spaces before CR", "VERDICT: PASS \t\r\n", "pass"},
		{"no final newline", "VERDICT: PASS", "pass"},
		{"leading space", " VERDICT: PASS\n", "unreadable"},
		{"duplicate pass", "VERDICT: PASS\nVERDICT: PASS\n", "unreadable"},
		{"following commentary", "VERDICT: PASS\nLooks good.\n", "unreadable"},
		{"empty", "", "unreadable"},
		{"blank", "\n\t \r\n", "unreadable"},
		{"NUL", "VERDICT: PASS\x00", "unreadable"},
		{"Unicode whitespace", "VERDICT: PASS\u00a0\n", "unreadable"},
		{"embedded CR", "VERDICT:\r PASS\n", "unreadable"},
		{"long findings", "VERDICT: CHANGES REQUIRED\n" + strings.Repeat("x", 1<<20), "changes"},
		{"invalid UTF8 findings", "VERDICT: CHANGES REQUIRED\n\xff", "changes"},
		{"DEL findings", "VERDICT: CHANGES REQUIRED\n\x7f", "changes"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := review.Verdict(strings.NewReader(tt.input))
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
	for b := range byte(32) {
		if b == '\t' || b == '\r' || b == '\n' {
			continue
		}
		t.Run(fmt.Sprintf("prohibited-%02x", b), func(t *testing.T) {
			t.Parallel()
			got, err := review.Verdict(strings.NewReader("VERDICT: CHANGES REQUIRED\n" + string([]byte{b})))
			if err != nil || got != "unreadable" {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

var errVerdictRead = errors.New("read failed")

type verdictReader struct{ partial bool }

func (r verdictReader) Read(p []byte) (int, error) {
	if r.partial {
		return copy(p, "VERDICT: PASS\n"), errVerdictRead
	}
	return 0, errVerdictRead
}

func TestVerdictReadFailure(t *testing.T) {
	t.Parallel()
	for _, input := range []io.Reader{verdictReader{}, verdictReader{partial: true}, io.MultiReader(strings.NewReader("VERDICT: PASS\n"), verdictReader{})} {
		got, err := review.Verdict(input)
		if !errors.Is(err, errVerdictRead) || got != "" {
			t.Fatalf("got %q, %v", got, err)
		}
	}
}
