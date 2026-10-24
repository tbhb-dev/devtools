// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tbhb-dev/devtools/internal/consumercontract"
)

// history builds the tagged releases every candidate test starts from:
// v1.0.0 has no contract, v1.1.0 is the baseline, and v1.2.0 is the bridge
// that accepts gamma. The tests below cover AC1 (notes match the contract),
// AC3 and AC8 (undeclared change fails, explicit none passes), AC6
// (unsupported schema), AC7 (skipped bridge), and AC9 (baseline matches the
// preceding reader).
func history(t *testing.T) string {
	t.Helper()
	requireProbeTools(t)
	return releases(t,
		release{"v1.0.0", "reader-v1", ""},
		release{"v1.1.0", "reader-v1", "contract-baseline.toml"},
		release{"v1.2.0", "reader-v2", "contract-bridge.toml"},
	)
}

// candidate commits a candidate tree on top of the history without tagging it.
func candidate(t *testing.T, repo, reader, contract string) {
	t.Helper()
	writeTree(t, repo, reader, contract)
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "candidate")
}

func headOf(t *testing.T, repo string) string {
	t.Helper()
	return strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
}

func TestValidateCandidateAcceptsAnUnchangedContractDeclaredNone(t *testing.T) {
	t.Parallel()
	repo := history(t)
	candidate(t, repo, "reader-v2", "contract-bridge.toml")
	got, err := consumercontract.ValidateCandidate(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if got.PreviousTag != "v1.2.0" || !got.Document.Transition.None {
		t.Fatalf("got %+v", got)
	}
}

func TestValidateCandidateAcceptsADeclaredBridgeCompatibleTransition(t *testing.T) {
	t.Parallel()
	repo := history(t)
	candidate(t, repo, "reader-v3", "contract-require.toml")
	if _, err := consumercontract.ValidateCandidate(t.Context(), repo); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCandidateRefusesAnUndeclaredChangeWithoutWriting(t *testing.T) {
	t.Parallel()
	repo := history(t)
	candidate(t, repo, "reader-v3", "contract-require-undeclared.toml")
	before := headOf(t, repo)
	_, err := consumercontract.ValidateCandidate(t.Context(), repo)
	if !errors.Is(err, consumercontract.ErrUndeclared) || !strings.Contains(err.Error(), ":gamma") {
		t.Fatalf("got %v", err)
	}
	if after := headOf(t, repo); after != before || git(t, repo, "status", "--porcelain") != "" {
		t.Fatal("validation changed the repository")
	}
	if tags := strings.Fields(git(t, repo, "tag", "--list")); len(tags) != 3 {
		t.Fatalf("tags %v", tags)
	}
}

func TestValidateCandidateRefusesATransitionThatSkipsItsBridge(t *testing.T) {
	t.Parallel()
	requireProbeTools(t)
	repo := releases(t,
		release{"v1.0.0", "reader-v1", ""},
		release{"v1.1.0", "reader-v1", "contract-baseline.toml"},
	)
	candidate(t, repo, "reader-v3", "contract-require-direct.toml")
	if _, err := consumercontract.ValidateCandidate(t.Context(), repo); !errors.Is(err, consumercontract.ErrNoOverlap) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateCandidateHoldsTheBaselineToTheObservedReader(t *testing.T) {
	t.Parallel()
	requireProbeTools(t)
	repo := releases(t, release{"v1.0.0", "reader-v1", ""})
	candidate(t, repo, "reader-v1", "contract-baseline.toml")
	if _, err := consumercontract.ValidateCandidate(t.Context(), repo); err != nil {
		t.Fatalf("matching baseline: %v", err)
	}
	// Over a stricter reader, a baseline that omits its requirements fails.
	hidden := releases(t, release{"v1.0.0", "reader-v1", ""})
	candidate(t, hidden, "reader-v3", "contract-baseline.toml")
	if _, err := consumercontract.ValidateCandidate(
		t.Context(),
		hidden,
	); !errors.Is(
		err,
		consumercontract.ErrContractDrift,
	) {
		t.Fatalf("baseline over a stricter reader: %v", err)
	}
}

func TestValidateCandidateRefusesContractsThatDriftFromTheirReader(t *testing.T) {
	t.Parallel()
	repo := history(t)
	candidate(t, repo, "reader-v2", "contract-require.toml")
	if _, err := consumercontract.ValidateCandidate(
		t.Context(),
		repo,
	); !errors.Is(
		err,
		consumercontract.ErrContractDrift,
	) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateCandidateRefusesMissingAndStaleNotes(t *testing.T) {
	t.Parallel()
	repo := history(t)
	candidate(t, repo, "reader-v2", "contract-bridge.toml")
	notes := filepath.Join(repo, filepath.FromSlash(consumercontract.NotesPath))
	writeFile(t, notes, "stale\n")
	if _, err := consumercontract.ValidateCandidate(
		t.Context(),
		repo,
	); !errors.Is(
		err,
		consumercontract.ErrNotesStale,
	) {
		t.Fatalf("stale: %v", err)
	}
	if err := os.Remove(notes); err != nil {
		t.Fatal(err)
	}
	if _, err := consumercontract.ValidateCandidate(
		t.Context(),
		repo,
	); !errors.Is(
		err,
		consumercontract.ErrNotesStale,
	) {
		t.Fatalf("missing: %v", err)
	}
}

func TestValidateCandidateRefusesWithoutAContractOrATag(t *testing.T) {
	t.Parallel()
	repo := history(t)
	candidate(t, repo, "reader-v2", "")
	if _, err := consumercontract.ValidateCandidate(
		t.Context(),
		repo,
	); !errors.Is(
		err,
		consumercontract.ErrMissingContract,
	) {
		t.Fatalf("no contract: %v", err)
	}
	untagged := t.TempDir()
	git(t, untagged, "init", "-q", "-b", "main")
	writeTree(t, untagged, "reader-v1", "contract-baseline.toml")
	git(t, untagged, "add", "-A")
	git(t, untagged, "commit", "-q", "-m", "first")
	if _, err := consumercontract.ValidateCandidate(t.Context(), untagged); !errors.Is(err, consumercontract.ErrNoTag) {
		t.Fatalf("no tag: %v", err)
	}
}

func TestValidateCandidateRejectsAnUnsupportedSchema(t *testing.T) {
	t.Parallel()
	repo := history(t)
	candidate(t, repo, "reader-v2", "contract-bridge.toml")
	path := filepath.Join(repo, filepath.FromSlash(consumercontract.ContractPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, strings.Replace(string(data), "schema_version = 1", "schema_version = 2", 1))
	_, err = consumercontract.ValidateCandidate(t.Context(), repo)
	if !errors.Is(err, consumercontract.ErrUnsupportedSchema) {
		t.Fatalf("got %v", err)
	}
}
