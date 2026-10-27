// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package fanout defines the read-only release fan-out foundation.
package fanout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// LedgerPath is read from the verified release commit, never the checkout.
	LedgerPath       = "release/consumers.json"
	maxEntries       = 100
	writeWindow      = 55 * time.Minute
	objectiveWindow  = time.Hour
	lateReportWindow = 5 * time.Minute
)

// Credential-free refusal categories never wrap subprocess output or ledger data.
var (
	ErrLedger  = errors.New("fanout: invalid ledger")
	ErrRelease = errors.New("fanout: release read or verification refused")
	ErrResults = errors.New("fanout: invalid results")
)

const (
	active   = "active"
	inactive = "inactive"
	exempt   = "exempt"
)

// Entry identifies a consumer and its active, inactive or exempt disposition.
type Entry struct {
	Repository  string `json:"repository"`
	Disposition string `json:"disposition"`
}

// ValidateLedger accepts only version 1 snapshots. Refusals never echo input.
func ValidateLedger(data []byte) ([]Entry, error) {
	if err := checkLedgerJSON(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return nil, ErrLedger
	}
	var snapshot struct {
		Version int     `json:"version"`
		Entries []Entry `json:"entries"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, ErrLedger
	}
	if decoder.Decode(new(any)) != io.EOF || snapshot.Version != 1 || snapshot.Entries == nil {
		return nil, ErrLedger
	}
	if err := validateEntries(snapshot.Entries); err != nil {
		return nil, err
	}
	return snapshot.Entries, nil
}

// checkLedgerJSON refuses nulls and repeated members before struct decoding can
// discard them. Member names follow the decoder's case-insensitive matching.
func checkLedgerJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil || token == nil {
		return ErrLedger
	}
	if token != json.Delim('{') && token != json.Delim('[') {
		return nil
	}
	seen := make(map[string]bool)
	for decoder.More() {
		if token == json.Delim('{') {
			if err = checkLedgerMember(decoder, seen); err != nil {
				return err
			}
		}
		if err = checkLedgerJSON(decoder); err != nil {
			return err
		}
	}
	if _, err = decoder.Token(); err != nil {
		return ErrLedger
	}
	return nil
}

func checkLedgerMember(decoder *json.Decoder, seen map[string]bool) error {
	member, err := decoder.Token()
	if err != nil {
		return ErrLedger
	}
	name, ok := member.(string)
	if !ok {
		return ErrLedger
	}
	name = strings.ToUpper(name)
	if seen[name] {
		return ErrLedger
	}
	seen[name] = true
	return nil
}

func validateEntries(entries []Entry) error {
	if len(entries) > maxEntries {
		return ErrLedger
	}
	repository := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.-]+$`)
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		key := strings.ToLower(entry.Repository)
		if !repository.MatchString(entry.Repository) || seen[key] ||
			(entry.Disposition != active && entry.Disposition != inactive && entry.Disposition != exempt) {
			return ErrLedger
		}
		seen[key] = true
	}
	return nil
}

// Release holds the pinned release identity and its authoritative ledger.
type Release struct {
	Commit   string
	TaggedAt time.Time
	Entries  []Entry
}

// Load verifies the tag through the existing release verifier before reading its
// commit's ledger. It refuses a tag that moves during verification. Subprocess
// diagnostics are deliberately discarded because they can contain credentials.
func Load(ctx context.Context, root, tag string) (Release, error) {
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	ref := "refs/tags/" + tag
	if _, err := readCommand(ctx, root, "git", "check-ref-format", ref); err != nil {
		return Release{}, err
	}
	object, err := readCommand(ctx, root, "git", "rev-parse", "--verify", ref)
	if err != nil {
		return Release{}, err
	}
	if _, err = readCommand(ctx, root, "bash", "tools/release-verify.sh", tag); err != nil {
		return Release{}, err
	}
	after, err := readCommand(ctx, root, "git", "rev-parse", "--verify", ref)
	if err != nil || object != after {
		return Release{}, ErrRelease
	}
	body, err := readCommand(ctx, root, "git", "cat-file", "tag", object)
	if err != nil {
		return Release{}, err
	}
	taggedAt, err := tagTime(body)
	if err != nil {
		return Release{}, err
	}
	commit, err := readCommand(ctx, root, "git", "rev-parse", object+"^{commit}")
	if err != nil {
		return Release{}, err
	}
	ledger, err := readCommand(ctx, root, "git", "show", commit+":"+LedgerPath)
	if err != nil {
		return Release{}, err
	}
	entries, err := ValidateLedger([]byte(ledger))
	if err != nil {
		return Release{}, err
	}
	return Release{Commit: commit, TaggedAt: taggedAt, Entries: entries}, nil
}

func readCommand(ctx context.Context, root, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return "", ErrRelease
	}
	return strings.TrimSpace(string(output)), nil
}

func tagTime(body string) (time.Time, error) {
	header, _, _ := strings.Cut(body, "\n\n")
	for line := range strings.SplitSeq(header, "\n") {
		if strings.HasPrefix(line, "tagger ") {
			fields := strings.Fields(line)
			const minimumTaggerFields = 3
			if len(fields) >= minimumTaggerFields {
				seconds, err := strconv.ParseInt(fields[len(fields)-2], 10, 64)
				if err == nil {
					return time.Unix(seconds, 0).UTC(), nil
				}
			}
		}
	}
	return time.Time{}, ErrRelease
}

// Decision supplies deadlines without scheduling work or recording results.
type Decision struct {
	WriteAllowed      bool
	Result            string
	MissedObjective   bool
	ReportingDeadline time.Time
}

// Decide closes writes at T+55m and computes the late-event reporting deadline.
func Decide(taggedAt, receivedAt time.Time) Decision {
	deadline := taggedAt.Add(objectiveWindow)
	if receivedAt.Before(taggedAt.Add(writeWindow)) {
		return Decision{WriteAllowed: true, ReportingDeadline: deadline}
	}
	missed := !receivedAt.Before(deadline)
	if missed {
		deadline = receivedAt.Add(lateReportWindow)
	}
	return Decision{Result: "late-event", MissedObjective: missed, ReportingDeadline: deadline}
}

// Result is the terminal outcome for one ledger entry.
type Result struct {
	Repository string
	Value      string
}

// Aggregate describes adoption, including its active denominator.
type Aggregate struct {
	Active  int
	Adopted int
	Outcome string
}

// Summarize refuses incomplete, repeated, unknown or disposition-mismatched
// results. Only current, existing, opened and newer count as adoption.
func Summarize(entries []Entry, results []Result) (Aggregate, error) {
	if err := validateEntries(entries); err != nil {
		return Aggregate{}, err
	}
	values, err := resultValues(results)
	if err != nil || len(values) != len(entries) {
		return Aggregate{}, ErrResults
	}
	aggregate := Aggregate{Outcome: "failed"}
	for _, entry := range entries {
		value := values[strings.ToLower(entry.Repository)]
		if !matchesDisposition(entry.Disposition, value) {
			return Aggregate{}, ErrResults
		}
		if entry.Disposition == active {
			aggregate.Active++
			if adopted(value) {
				aggregate.Adopted++
			}
		}
	}
	if aggregate.Active == 0 {
		aggregate.Outcome = "no-active-entries"
	} else if aggregate.Adopted == aggregate.Active {
		aggregate.Outcome = "passed"
	}
	return aggregate, nil
}

func matchesDisposition(disposition, value string) bool {
	if disposition == active {
		return value != "" && value != inactive && value != exempt
	}
	return value == disposition
}

func resultValues(results []Result) (map[string]string, error) {
	values := make(map[string]string, len(results))
	for _, result := range results {
		key := strings.ToLower(result.Repository)
		if values[key] != "" || !validResult(result.Value) {
			return nil, ErrResults
		}
		values[key] = result.Value
	}
	return values, nil
}

func adopted(value string) bool {
	return value == "current" || value == "existing" || value == "opened" || value == "newer"
}

func validResult(value string) bool {
	switch value {
	case "current", "existing", "opened", "newer", "inactive", "exempt", "late-event", "deadline",
		"prepare-failed", "publish-failed", "access-refused", "scan-refused", "conflict", "uncertain":
		return true
	default:
		return false
	}
}
