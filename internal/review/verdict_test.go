// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package review_test

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/review"
)

func TestVerdict(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, reply, want string
	}{
		{"bare pass with blank lines", "\nVERDICT: PASS\n\n", "pass"},
		{"surrounding commentary", "Looks good.\nVERDICT: PASS\n", "unreadable"},
		{"quoted pass in findings", "VERDICT: CHANGES REQUIRED\n\n1. [form] quoted VERDICT: PASS\n", "changes"},
		{"control byte", "VERDICT: PASS\x01\n", "unreadable"},
		{"CRLF", "\r\nVERDICT: PASS\r\n\r\n", "pass"},
		{"trailing spaces and tabs", " \t\nVERDICT: PASS \t\r\n\t\n", "pass"},
		{"CR before spaces", "VERDICT: PASS\r \n", "unreadable"},
		{"repeated CR", "VERDICT: PASS\r\r\n", "unreadable"},
		{"leading space", " VERDICT: PASS\n", "unreadable"},
		{"multiple substantive lines", "VERDICT: PASS\nVERDICT: PASS\n", "unreadable"},
		{"comment after pass", "VERDICT: PASS\nLooks good.\n", "unreadable"},
		{"empty", "", "unreadable"},
		{"blank", "\n \t\r\n", "unreadable"},
		{"no final newline", "VERDICT: PASS", "pass"},
		{"changes alone", "VERDICT: CHANGES REQUIRED", "changes"},
		{"comment before changes", "Comment\nVERDICT: CHANGES REQUIRED\n", "unreadable"},
		{"long finding", "VERDICT: CHANGES REQUIRED\n" + strings.Repeat("x", 128*1024), "changes"},
		{"Unicode whitespace", "VERDICT: PASS\u00a0\n", "unreadable"},
		{"DEL is permitted in findings", "VERDICT: CHANGES REQUIRED\n\x7f", "changes"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := review.Verdict(strings.NewReader(tt.reply))
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestVerdictProhibitedBytes(t *testing.T) {
	t.Parallel()
	for b := range byte(32) {
		if b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		got, err := review.Verdict(strings.NewReader("VERDICT: CHANGES REQUIRED\n" + string(b)))
		require.NoError(t, err)
		require.Equal(t, "unreadable", got, "byte %d", b)
	}
}

func TestVerdictReadFailure(t *testing.T) {
	t.Parallel()
	want := io.ErrUnexpectedEOF
	got, err := review.Verdict(iotest.ErrReader(want))
	require.ErrorIs(t, err, want)
	require.Empty(t, got)

	got, err = review.Verdict(iotest.TimeoutReader(strings.NewReader("VERDICT: PASS\n")))
	require.ErrorIs(t, err, iotest.ErrTimeout)
	require.Empty(t, got, "partial PASS must never be accepted")
}
