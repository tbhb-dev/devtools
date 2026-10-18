// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package checks evaluates saved GitHub pull request rollup data without I/O.
package checks

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Outcome is also the command's stable exit status.
type Outcome int

const (
	Success Outcome = iota
	Failed
	Pending
	MissingRequired
	Empty
	Unreadable
	HeadMoved
)

const (
	checkRun      = "CheckRun"
	statusContext = "StatusContext"
)

const (
	goodVerdict = iota
	pendingVerdict
	badVerdict
)

var errInput = errors.New("invalid rollup")

// Result describes an evaluation. Names are sorted for reproducible output.
type Result struct {
	Outcome Outcome  `json:"outcome"`
	Names   []string `json:"names,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// Rollup is the saved object returned by the GitHub CLI. Request headRefOid as well
// as statusCheckRollup when evaluating an expected head.
type Rollup struct {
	Head   string  `json:"headRefOid"`
	Checks []Entry `json:"statusCheckRollup"`
}

// Entry represents either a CheckRun or a legacy StatusContext.
type Entry struct {
	Type       string `json:"__typename"`
	Workflow   string `json:"workflowName"`
	Name       string `json:"name"`
	Context    string `json:"context"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
	Started    string `json:"startedAt"`
	Created    string `json:"createdAt"`
	when       time.Time
}

// Parse rejects malformed or ambiguous input rather than interpreting it as green.
func Parse(data []byte) (Rollup, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Rollup{}, fmt.Errorf("%w: %w", errInput, err)
	}
	value, ok := raw["statusCheckRollup"]
	if !ok || string(value) == "null" {
		return Rollup{}, fmt.Errorf("%w: missing statusCheckRollup array", errInput)
	}
	var r Rollup
	if err := json.Unmarshal(data, &r); err != nil {
		return Rollup{}, fmt.Errorf("%w: %w", errInput, err)
	}
	for i, e := range r.Checks {
		switch e.Type {
		case checkRun:
			if e.Name == "" {
				return Rollup{}, fmt.Errorf("%w: check name is empty", errInput)
			}
		case statusContext:
			if e.Context == "" {
				return Rollup{}, fmt.Errorf("%w: status context is empty", errInput)
			}
		default:
			return Rollup{}, fmt.Errorf("%w: unknown entry type %q", errInput, e.Type)
		}
		when, err := timestamp(e)
		if err != nil {
			return Rollup{}, fmt.Errorf("%w: %w", errInput, err)
		}
		r.Checks[i].when = when
	}
	return r, nil
}

func timestamp(e Entry) (time.Time, error) {
	s := e.Started
	if e.Type == statusContext {
		s = e.Created
	}
	if s == "" || s == "0001-01-01T00:00:00Z" {
		return time.Time{}, nil
	}
	when, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %w", errInput, err)
	}
	return when, nil
}

// Evaluate parses and judges a snapshot. Precedence is unreadable, head moved,
// empty, missing required, failed, pending, success. Required names match exactly;
// every workflow with that name must succeed.
func Evaluate(data []byte, expectedHead string, required []string) Result {
	r, err := Parse(data)
	if err != nil {
		return Result{Outcome: Unreadable, Error: err.Error()}
	}
	if expectedHead != "" {
		if r.Head == "" {
			return Result{Outcome: Unreadable, Error: "missing headRefOid"}
		}
		if r.Head != expectedHead {
			return Result{Outcome: HeadMoved}
		}
	}
	if len(r.Checks) == 0 {
		return Result{Outcome: Empty}
	}
	needed := make(map[string]bool)
	for _, name := range required {
		needed[name] = true
	}
	latest := selectLatest(r.Checks, needed)

	missing := make(map[string]bool)
	for name := range needed {
		missing[name] = true
	}
	bad, waiting := make(map[string]bool), make(map[string]bool)
	for k, e := range latest {
		delete(missing, k.name)
		switch verdict(e, needed[k.name]) {
		case badVerdict:
			bad[k.name] = true
		case pendingVerdict:
			waiting[k.name] = true
		}
	}
	if len(missing) > 0 {
		return named(MissingRequired, missing)
	}
	if len(bad) > 0 {
		return named(Failed, bad)
	}
	if len(waiting) > 0 {
		return named(Pending, waiting)
	}
	return Result{Outcome: Success}
}

func verdict(e Entry, required bool) int {
	state := e.Conclusion
	if e.Type == statusContext {
		state = e.State
	} else if e.Status != "COMPLETED" {
		switch e.Status {
		case "QUEUED", "IN_PROGRESS", "WAITING", "REQUESTED", "PENDING":
			return pendingVerdict
		default:
			return badVerdict
		}
	}
	switch state {
	case "SUCCESS":
		return goodVerdict
	case "NEUTRAL", "SKIPPED":
		if !required {
			return goodVerdict
		}
	case "PENDING", "EXPECTED", "":
		return pendingVerdict
	}
	return badVerdict
}

func named(outcome Outcome, names map[string]bool) Result {
	r := Result{Outcome: outcome}
	for name := range names {
		r.Names = append(r.Names, name)
	}
	sort.Strings(r.Names)
	return r
}

func selectLatest(entries []Entry, needed map[string]bool) map[key]Entry {
	latest := make(map[key]Entry)
	for _, e := range entries {
		name := e.Name
		if e.Type == statusContext {
			name = e.Context
		}
		k := key{e.Type, e.Workflow, name}
		old, ok := latest[k]
		t, previous := e.when, old.when
		// A run that hasn't started supersedes started runs. Equal timestamps
		// keep the worse verdict, independent of input order.
		newer := !t.IsZero() && !previous.IsZero() && t.After(previous)
		if !ok || (t.IsZero() && !previous.IsZero()) || newer ||
			(t.Equal(previous) && verdict(e, needed[name]) > verdict(old, needed[name])) {
			latest[k] = e
		}
	}
	return latest
}

type key struct{ kind, workflow, name string }
