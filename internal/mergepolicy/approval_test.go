// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package mergepolicy_test

import (
	"encoding/json"
	"testing"

	"github.com/tbhb-dev/devtools/internal/mergepolicy"
)

func TestApproved(t *testing.T) {
	t.Parallel()
	type review struct {
		login, state, head, kind, at string
	}
	approved := review{"reviewer", "APPROVED", "head", "Bot", "2026-01-01"}
	for _, tt := range []struct {
		name, login, kind string
		reviews           []review
		want              bool
	}{
		{"exact head", "reviewer", "", []review{approved}, true},
		{"Bot matches", "reviewer", "Bot", []review{approved}, true},
		{"Bot cannot satisfy User", "reviewer", "User", []review{approved}, false},
		{"User matches", "reviewer", "User", []review{{"reviewer", "APPROVED", "head", "User", "1"}}, true},
		{"User cannot satisfy Bot", "reviewer", "Bot", []review{{"reviewer", "APPROVED", "head", "User", "1"}}, false},
		{"missing type fails typed", "reviewer", "Bot", []review{{"reviewer", "APPROVED", "head", "", "1"}}, false},
		{"missing type passes untyped", "reviewer", "", []review{{"reviewer", "APPROVED", "head", "", "1"}}, true},
		{"empty login", "", "", []review{approved}, false},
		{"absent approval", "reviewer", "", nil, false},
		{"other login", "owner", "", []review{approved}, false},
		{"stale head", "reviewer", "", []review{{"reviewer", "APPROVED", "old", "Bot", "1"}}, false},
		{"later stale approval", "reviewer", "", []review{approved, {"reviewer", "APPROVED", "old", "Bot", "2026-01-02"}}, false},
		{"later changes requested", "reviewer", "", []review{approved, {"reviewer", "CHANGES_REQUESTED", "old", "Bot", "2026-01-02"}}, false},
		{"dismissal", "reviewer", "", []review{approved, {"reviewer", "DISMISSED", "head", "Bot", "2026-01-02"}}, false},
		{"comment cannot supersede", "reviewer", "", []review{approved, {"reviewer", "COMMENTED", "head", "Bot", "2026-01-02"}}, true},
		{"pending cannot supersede", "reviewer", "", []review{approved, {"reviewer", "PENDING", "head", "Bot", "2026-01-02"}}, true},
		{"owner independent", "owner", "", []review{approved, {"owner", "APPROVED", "head", "User", "1"}}, true},
		{"timestamp beats position", "reviewer", "", []review{{"reviewer", "CHANGES_REQUESTED", "head", "Bot", "2026-01-02"}, approved}, false},
		{"equal timestamp withdrawal last", "reviewer", "", []review{approved, {"reviewer", "CHANGES_REQUESTED", "head", "Bot", approved.at}}, false},
		{"equal timestamp approval last", "reviewer", "", []review{{"reviewer", "CHANGES_REQUESTED", "head", "Bot", approved.at}, approved}, true},
		{"later wrong type wins", "reviewer", "Bot", []review{approved, {"reviewer", "APPROVED", "head", "User", "2026-01-02"}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reviews := make([]map[string]any, 0, len(tt.reviews))
			for _, r := range tt.reviews {
				reviews = append(reviews, map[string]any{
					"author":      map[string]string{"login": r.login, "__typename": r.kind},
					"state":       r.state,
					"commit":      map[string]string{"oid": r.head},
					"submittedAt": r.at,
				})
			}
			data, err := json.Marshal(map[string]any{"reviews": reviews})
			if err != nil {
				t.Fatal(err)
			}
			got, err := mergepolicy.Approved(data, tt.login, "head", tt.kind)
			if err != nil || got != tt.want {
				t.Fatalf("Approved = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestApprovedInvalid(t *testing.T) {
	t.Parallel()
	for _, input := range []string{`invalid`, `null`, `{}`, `{"reviews":null}`, `{"reviews":{}}`, `{"reviews":[{"state":1}]}`, `{"reviews":[]} {}`} {
		if _, err := mergepolicy.Approved([]byte(input), "reviewer", "head", ""); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
	for _, args := range [][2]string{{"", ""}, {"head", "App"}} {
		if _, err := mergepolicy.Approved([]byte(`{"reviews":[]}`), "reviewer", args[0], args[1]); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}
