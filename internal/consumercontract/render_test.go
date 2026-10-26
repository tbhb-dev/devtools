// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract_test

import (
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/consumercontract"
)

// AC1: the notes have a dedicated section that says none or lists each one.
func TestRenderStatesNoneOrListsEveryTransition(t *testing.T) {
	t.Parallel()
	none := consumercontract.Render(parseFixture(t, "contract-bridge.toml"))
	for _, want := range []string{"## Transitions", "This release doesn't require a consumer change."} {
		if !strings.Contains(none, want) {
			t.Errorf("none notes lack %q", want)
		}
	}
	listed := consumercontract.Render(parseFixture(t, "contract-require.toml"))
	if strings.Contains(listed, "doesn't require a consumer change") {
		t.Error("notes with a transition also say there is none")
	}
	// AC2: the notes include each field the contract exposes.
	for _, want := range []string{
		"gamma", consumercontract.ConfFile, "the key is absent", "`on`", "v1.2.0",
		"The reader accepts the conf and gamma is set.", "Delete the gamma line.", "Schema version 1",
	} {
		if !strings.Contains(listed, want) {
			t.Errorf("notes lack %q", want)
		}
	}
}

func TestRenderIsDeterministicAndNeverWrapsParagraphs(t *testing.T) {
	t.Parallel()
	doc := parseFixture(t, "contract-require.toml")
	text := consumercontract.Render(doc)
	if text != consumercontract.Render(doc) {
		t.Fatal("render differs between calls")
	}
	if !strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\n\n") {
		t.Error("notes must end in exactly one newline")
	}
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "  ") {
			t.Errorf("continuation line looks like a wrapped paragraph: %q", line)
		}
	}
}
