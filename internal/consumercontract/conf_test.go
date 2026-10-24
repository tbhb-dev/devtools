// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract_test

import (
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/consumercontract"
)

// pinnedTarget is a target contract whose gamma accepts only the value on.
func pinnedTarget(t *testing.T) *consumercontract.Document {
	t.Helper()
	text := fixtureText(t, "contract-require.toml")
	text = strings.Replace(
		text,
		"name = \"gamma\"\nrequired = true\naccepts = [\"empty\", \"nonempty\"]\nsample = \"x\"",
		"name = \"gamma\"\nrequired = true\naccepts = [\"value\"]\nvalue = \"on\"\nsample = \"on\"",
		1,
	)
	doc, err := consumercontract.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestAppliesJudgesTheConsumersCurrentState(t *testing.T) {
	t.Parallel()
	target := pinnedTarget(t)
	absent := target.Transition.Entries[0]
	rejected := absent
	rejected.Kind = consumercontract.KindRequiredValueChanged
	rejected.AppliesWhen = consumercontract.WhenValueRejected
	for name, test := range map[string]struct {
		entry consumercontract.Entry
		conf  string
		want  bool
	}{
		"absent key":               {absent, "alpha = x\n", true},
		"present key":              {absent, "gamma = on\n", false},
		"commented key is absent":  {absent, "# gamma = on\n", true},
		"rejected value":           {rejected, "gamma = off\n", true},
		"accepted value":           {rejected, "gamma = on\n", false},
		"rejected but not present": {rejected, "alpha = x\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := consumercontract.Applies(test.entry, test.conf, target); got != test.want {
				t.Fatalf("got %v", got)
			}
		})
	}
	gone := rejected
	gone.Key = "retired"
	if !consumercontract.Applies(gone, "retired = 1\n", target) {
		t.Error("a key the target no longer lists is always rejected")
	}
}

func TestEditChangesOnlyTheNamedKey(t *testing.T) {
	t.Parallel()
	target := pinnedTarget(t)
	entry := target.Transition.Entries[0]
	retired := entry
	retired.Key = "retired"
	retired.TargetValue = ""
	for name, test := range map[string]struct {
		entry      consumercontract.Entry
		conf, want string
	}{
		"appends to an empty conf":         {entry, "", "gamma = on\n"},
		"appends after a final newline":    {entry, "alpha = x\n", "alpha = x\ngamma = on\n"},
		"adds the missing newline":         {entry, "alpha = x", "alpha = x\ngamma = on\n"},
		"replaces in place":                {entry, "alpha = x\ngamma = off\nbeta = y\n", "alpha = x\ngamma = on\nbeta = y\n"},
		"collapses a repeated key":         {entry, "gamma = a\nalpha = x\ngamma = b\n", "gamma = on\nalpha = x\n"},
		"keeps comments":                   {entry, "# gamma = off\nalpha = x\n", "# gamma = off\nalpha = x\ngamma = on\n"},
		"removes a retired key":            {retired, "alpha = x\nretired = 1\nbeta = y\n", "alpha = x\nbeta = y\n"},
		"retiring an absent key is a noop": {retired, "alpha = x\n", "alpha = x\n"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := consumercontract.Edit(test.conf, test.entry, target); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderCoversEveryTransitionKind(t *testing.T) {
	t.Parallel()
	doc := pinnedTarget(t)
	doc.Transition.Entries = append(
		doc.Transition.Entries,
		consumercontract.Entry{
			Kind:         consumercontract.KindRequiredValueChanged,
			File:         consumercontract.ConfFile,
			Key:          "gamma",
			AppliesWhen:  consumercontract.WhenValueRejected,
			TargetValue:  "on",
			AfterRelease: "v1.2.0",
			Verify:       "v",
			Rollback:     "r",
		},
		consumercontract.Entry{
			Kind:         consumercontract.KindAcceptedRemoved,
			File:         consumercontract.ConfFile,
			Key:          "retired",
			AppliesWhen:  consumercontract.WhenValueRejected,
			AfterRelease: "v1.2.0",
			Verify:       "v",
			Rollback:     "r",
		},
	)
	text := consumercontract.Render(doc)
	for _, want := range []string{"Required key added", "Required value changed", "Accepted value removed", "exactly `on`", "the key holds a value the new reader rejects"} {
		if !strings.Contains(text, want) {
			t.Errorf("notes lack %q", want)
		}
	}
}
