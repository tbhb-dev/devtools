// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"testing"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestSkillsCapabilityAndUsage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args        []string
		code        int
		out, stderr string
	}{
		{[]string{"generate-skills", "--capability"}, 0, "generate-skills-v1\n", ""},
		{[]string{"generate-skills", "invalid"}, 2, "", "usage: generate-skills.sh [--check]\n"},
	} {
		t.Run(tc.args[1], func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			got := agentrepo.Run(tc.args, &bytes.Buffer{}, &out, &stderr)
			if got != tc.code || out.String() != tc.out || stderr.String() != tc.stderr {
				t.Fatalf("%d %q %q", got, &out, &stderr)
			}
		})
	}
}
