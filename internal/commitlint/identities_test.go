// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package commitlint_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/commitlint"
)

func TestIdentityRestrictions(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, value string
		identities  commitlint.Identities
		denied      bool
	}{
		{"default permits bots", "Release <release[bot]@users.noreply.github.com>", commitlint.Identities{}, false},
		{"exact agent", "Agent <agent@example.com>", commitlint.Identities{Agents: []commitlint.Identity{{Email: "agent@example.com"}}}, true},
		{"case insensitive email", "Agent <AGENT@EXAMPLE.COM>", commitlint.Identities{Agents: []commitlint.Identity{{Email: "agent@example.com"}}}, true},
		{"glob literal brackets", "Bot <release[bot]@users.noreply.github.com>", commitlint.Identities{Agents: []commitlint.Identity{{Email: "*[bot]@users.noreply.github.com"}}}, true},
		{"glob does not treat brackets as class", "Bot <robot@users.noreply.github.com>", commitlint.Identities{Agents: []commitlint.Identity{{Email: "*[bot]@users.noreply.github.com"}}}, false},
		{"optional name matches", "Agent <agent@example.com>", commitlint.Identities{Agents: []commitlint.Identity{{Email: "agent@*", Name: "Agent"}}}, true},
		{"different name", "Person <agent@example.com>", commitlint.Identities{Agents: []commitlint.Identity{{Email: "agent@*", Name: "Agent"}}}, false},
		{"allowed person", "Person <p@example.com>", commitlint.Identities{SignOff: []commitlint.Identity{{Email: "?@example.com", Name: "Person"}}}, false},
		{"unlisted person", "Other <other@example.com>", commitlint.Identities{SignOff: []commitlint.Identity{{Email: "person@example.com"}}}, true},
		{"deny wins", "Agent <agent@example.com>", commitlint.Identities{Agents: []commitlint.Identity{{Email: "agent@*"}}, SignOff: []commitlint.Identity{{Email: "*"}}}, true},
		{"malformed restricted signoff", "Agent", commitlint.Identities{Agents: []commitlint.Identity{{Email: "agent@*"}}}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := commitlint.DefaultConfig()
			c.Identities = tt.identities
			if got := c.CheckSignoff(tt.value); (got != nil) != tt.denied {
				t.Fatalf("CheckSignoff = %v, denied = %v", got, tt.denied)
			}
		})
	}
}

func TestIdentityConfigurationAndCommand(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".commit-lint.toml"), `require_signed_off_by = false
[identities]
agents = [{email = "agent@*"}]
sign_off = [{email = "person@example.com", name = "Person"}]
`)
	for _, tt := range []struct {
		args    []string
		message string
		status  int
		want    string
	}{
		{[]string{"--check-signoff", "Agent <agent@example.com>"}, "", 1, "responsible person"},
		{[]string{"--check-signoff", "Person <person@example.com>"}, "", 0, ""},
		{[]string{"--check-signoff", "Other <other@example.com>"}, "", 1, "not allowed"},
		{[]string{"--check-signoff", "Person <person@example.com>", "file"}, "", 2, "takes no message file"},
		{[]string{"--rules", "trailers"}, "fix: a thing\n\nSigned-off-by: Person <person@example.com>\nSigned-off-by: Agent <agent@example.com>", 1, "line 4: Signed-off-by: Agent"},
		{[]string{"--rules", "trailers"}, "fix: a thing\n\nSigned-off-by: Person <person@example.com>", 0, ""},
	} {
		var out bytes.Buffer
		status := commitlint.Run(tt.args, strings.NewReader(tt.message), &out, dir, func(string) string { return "" })
		if status != tt.status || !strings.Contains(out.String(), tt.want) {
			t.Fatalf("%v: (%d, %s)", tt.args, status, out.String())
		}
	}
}
