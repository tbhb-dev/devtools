// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package checks_test

import (
	"encoding/json"
	"testing"

	"github.com/tbhb-dev/devtools/internal/checks"
)

func TestEvaluate(t *testing.T) {
	t.Parallel()
	run := func(workflow, state, started string) checks.Entry {
		return checks.Entry{
			Type:       "CheckRun",
			Workflow:   workflow,
			Name:       "test",
			Status:     "COMPLETED",
			Conclusion: state,
			Started:    started,
		}
	}
	old := "2026-10-01T01:00:00Z"
	recent := "2026-10-01T02:00:00Z"
	cancelled := run("ci", "CANCELLED", old)
	success := run("ci", "SUCCESS", recent)
	queued := run("ci", "", "")
	queued.Status = "QUEUED"
	unknown := success
	unknown.Status = "ALIEN"
	status := func(state, created string) checks.Entry {
		return checks.Entry{Type: "StatusContext", Context: "test", State: state, Created: created}
	}
	tests := []struct {
		name     string
		entries  []checks.Entry
		required []string
		expected string
		want     checks.Outcome
	}{
		{name: "empty", want: checks.Empty},
		{name: "success", entries: []checks.Entry{success}, want: checks.Success},
		{name: "superseded cancellation", entries: []checks.Entry{cancelled, success}, want: checks.Success},
		{name: "reordered cancellation", entries: []checks.Entry{success, cancelled}, want: checks.Success},
		{name: "lone cancellation", entries: []checks.Entry{cancelled}, want: checks.Failed},
		{
			name:    "new failure",
			entries: []checks.Entry{success, run("ci", "FAILURE", "2026-10-01T03:00:00Z")},
			want:    checks.Failed,
		},
		{name: "queued rerun", entries: []checks.Entry{success, queued}, want: checks.Pending},
		{
			name: "zero start rerun",
			entries: []checks.Entry{
				success,
				{Type: "CheckRun", Workflow: "ci", Name: "test", Status: "QUEUED", Started: "0001-01-01T00:00:00Z"},
			},
			want: checks.Pending,
		},
		{
			name: "pending tie",
			entries: []checks.Entry{
				success,
				{Type: "CheckRun", Workflow: "ci", Name: "test", Status: "IN_PROGRESS", Started: recent},
			},
			want: checks.Pending,
		},
		{
			name:     "required across workflows",
			entries:  []checks.Entry{success, run("other", "SKIPPED", old)},
			required: []string{"test"},
			want:     checks.Failed,
		},
		{
			name:    "reordered statuses",
			entries: []checks.Entry{status("SUCCESS", recent), status("ERROR", old)},
			want:    checks.Success,
		},
		{name: "status unknown", entries: []checks.Entry{status("ALIEN", old)}, want: checks.Failed},
		{name: "queued first", entries: []checks.Entry{queued, success}, want: checks.Pending},
		{name: "tie", entries: []checks.Entry{success, run("ci", "CANCELLED", recent)}, want: checks.Failed},
		{name: "reverse tie", entries: []checks.Entry{run("ci", "CANCELLED", recent), success}, want: checks.Failed},
		{
			name:    "separate workflows",
			entries: []checks.Entry{success, run("other", "CANCELLED", old)},
			want:    checks.Failed,
		},
		{name: "neutral", entries: []checks.Entry{run("ci", "NEUTRAL", old)}, want: checks.Success},
		{name: "skipped", entries: []checks.Entry{run("ci", "SKIPPED", old)}, want: checks.Success},
		{
			name:     "required neutral",
			entries:  []checks.Entry{run("ci", "NEUTRAL", old)},
			required: []string{"test"},
			want:     checks.Failed,
		},
		{
			name:     "required skipped",
			entries:  []checks.Entry{run("ci", "SKIPPED", old)},
			required: []string{"test"},
			want:     checks.Failed,
		},
		{name: "required success", entries: []checks.Entry{success}, required: []string{"test"}, want: checks.Success},
		{
			name:     "required exact",
			entries:  []checks.Entry{success},
			required: []string{"te*"},
			want:     checks.MissingRequired,
		},
		{
			name:     "missing required",
			entries:  []checks.Entry{success},
			required: []string{"lint"},
			want:     checks.MissingRequired,
		},
		{name: "head moved", entries: []checks.Entry{success}, expected: "other", want: checks.HeadMoved},
		{name: "matching head", entries: []checks.Entry{success}, expected: "head", want: checks.Success},
		{name: "unknown conclusion", entries: []checks.Entry{run("ci", "ALIEN", old)}, want: checks.Failed},
		{name: "unknown status", entries: []checks.Entry{unknown}, want: checks.Failed},
		{name: "status success", entries: []checks.Entry{status("SUCCESS", old)}, want: checks.Success},
		{name: "status pending", entries: []checks.Entry{status("PENDING", old)}, want: checks.Pending},
		{name: "status error", entries: []checks.Entry{status("ERROR", old)}, want: checks.Failed},
		{
			name:    "status rerun",
			entries: []checks.Entry{status("ERROR", old), status("SUCCESS", recent)},
			want:    checks.Success,
		},
		{
			name:    "status tie",
			entries: []checks.Entry{status("SUCCESS", old), status("ERROR", old)},
			want:    checks.Failed,
		},
		{name: "separate entry types", entries: []checks.Entry{success, status("ERROR", old)}, want: checks.Failed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entries := tt.entries
			if entries == nil {
				entries = []checks.Entry{}
			}
			data, err := json.Marshal(checks.Rollup{Head: "head", Checks: entries})
			if err != nil {
				t.Fatal(err)
			}
			got := checks.Evaluate(data, tt.expected, tt.required)
			if got.Outcome != tt.want {
				t.Fatalf("got %+v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnreadable(t *testing.T) {
	t.Parallel()
	for _, input := range []string{``, `null`, `{}`, `[]`, `{"statusCheckRollup":null}`, `{"statusCheckRollup":{}}`, `{"statusCheckRollup":[{}]}`, `{"statusCheckRollup":[{"__typename":"CheckRun"}]}`, `{"statusCheckRollup":[{"__typename":"StatusContext"}]}`, `{"statusCheckRollup":[{"__typename":"CheckRun","name":"test","startedAt":"bad"}]}`} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if got := checks.Evaluate([]byte(input), "", nil); got.Outcome != checks.Unreadable || got.Error == "" {
				t.Fatalf("got %+v", got)
			}
		})
	}
	if got := checks.Evaluate([]byte(`{"statusCheckRollup":[]}`), "head", nil); got.Outcome != checks.Unreadable {
		t.Fatal(got)
	}
}

func TestNames(t *testing.T) {
	t.Parallel()
	input := []byte(`{"statusCheckRollup":[{"__typename":"StatusContext","context":"ci","state":"SUCCESS"}]}`)
	got := checks.Evaluate(input, "", []string{"z", "a", "z"})
	if got.Outcome != checks.MissingRequired || len(got.Names) != 2 || got.Names[0] != "a" || got.Names[1] != "z" {
		t.Fatal(got)
	}
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{`{}`, `null`, `{"statusCheckRollup":[]}`, `{"statusCheckRollup":[{"__typename":"StatusContext","context":"ci","state":"SUCCESS"}]}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := checks.Parse(data)
		if err != nil {
			return
		}
		encoded, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = checks.Parse(encoded); err != nil {
			t.Fatal(err)
		}
		_ = checks.Evaluate(data, "", nil)
	})
}
