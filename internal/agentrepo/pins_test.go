// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func TestUpdatePinsCapability(t *testing.T) {
	t.Parallel()
	var out, stderr bytes.Buffer
	if code := agentrepo.Run([]string{"update-pins", "--help"}, nil, &out, &stderr); code != 0 {
		t.Fatalf("capability unavailable: %d %s", code, &stderr)
	}
}

func TestUpdatePinsRefusesMissingRequiredTools(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", "")
	var out, stderr bytes.Buffer
	code := agentrepo.Run([]string{"update-pins", "0.11.0"}, nil, &out, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "guard-markdown") {
		t.Fatalf("unexpected result: %d %s", code, &stderr)
	}
}
