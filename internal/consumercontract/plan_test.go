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

// published is a remote whose tags run: a release with no contract, the
// baseline, the bridge that accepts gamma, and the release that requires it.
func published(t *testing.T) string {
	t.Helper()
	requireProbeTools(t)
	return releases(t,
		release{"v1.0.0", "reader-v1", ""},
		release{"v1.1.0", "reader-v1", "contract-baseline.toml"},
		release{"v1.2.0", "reader-v2", "contract-bridge.toml"},
		release{"v1.3.0", "reader-v3", "contract-require.toml"},
	)
}

// consumer checks the remote out at tag and gives it conf as its working conf.
func consumer(t *testing.T, remote, tag, conf string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "clone", "-q", remote, ".")
	git(t, dir, "checkout", "-q", tag)
	writeFile(t, filepath.Join(dir, filepath.FromSlash(consumercontract.ConfFile)), conf)
	return dir
}

func confOf(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(consumercontract.ConfFile)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// AC4: a consumer that has the bridge gets the transition in the same tree
// as the bump, and both readers accept that tree.
func TestBumpAppliesTheTransitionAfterItsBridge(t *testing.T) {
	t.Parallel()
	remote := published(t)
	dir := consumer(t, remote, "v1.2.0", "# keep\nalpha = x\n")
	state := t.TempDir()
	plan, err := consumercontract.Prepare(t.Context(), dir, remote, "v1.3.0", state)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Key != "gamma" || !plan.Changed {
		t.Fatalf("plan %+v", plan)
	}
	changed, err := consumercontract.Apply(dir, state)
	if err != nil || !changed {
		t.Fatalf("apply: %v %v", changed, err)
	}
	if got := confOf(t, dir); got != "# keep\nalpha = x\ngamma = on\n" {
		t.Fatalf("conf %q", got)
	}
	if err = consumercontract.Verify(t.Context(), dir, state); err != nil {
		t.Fatal(err)
	}
}

// AC5: a consumer that already satisfies the transition isn't edited at all.
func TestBumpLeavesASatisfiedConsumerUntouched(t *testing.T) {
	t.Parallel()
	remote := published(t)
	conf := "alpha   =   x\n\n# a comment\ngamma = on\n"
	dir := consumer(t, remote, "v1.2.0", conf)
	state := t.TempDir()
	plan, err := consumercontract.Prepare(t.Context(), dir, remote, "v1.3.0", state)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 0 || plan.Changed {
		t.Fatalf("plan %+v", plan)
	}
	changed, err := consumercontract.Apply(dir, state)
	if err != nil || changed {
		t.Fatalf("apply: %v %v", changed, err)
	}
	if got := confOf(t, dir); got != conf {
		t.Fatalf("conf changed to %q", got)
	}
	if err = consumercontract.Verify(t.Context(), dir, state); err != nil {
		t.Fatal(err)
	}
}

// AC10: a consumer that skips the bridge is refused before any edit, and the
// refusal names the release to adopt first. The consumer's own reader is the
// evidence, not a neighboring release's.
func TestBumpRefusesAConsumerWhoseReaderSkipsTheBridge(t *testing.T) {
	t.Parallel()
	remote := published(t)
	dir := consumer(t, remote, "v1.1.0", "alpha = x\n")
	_, err := consumercontract.Prepare(t.Context(), dir, remote, "v1.3.0", t.TempDir())
	if !errors.Is(err, consumercontract.ErrRefused) || !strings.Contains(err.Error(), "v1.2.0") {
		t.Fatalf("got %v", err)
	}
	if got := confOf(t, dir); got != "alpha = x\n" {
		t.Fatalf("a refused bump edited the conf: %q", got)
	}
}

// AC10: a consumer that predates the baseline is sent to the baseline.
func TestBumpRefusesAConsumerThatPredatesTheBaseline(t *testing.T) {
	t.Parallel()
	remote := published(t)
	dir := consumer(t, remote, "v1.0.0", "alpha = x\n")
	for _, tag := range []string{"v1.2.0", "v1.3.0"} {
		_, err := consumercontract.Prepare(t.Context(), dir, remote, tag, t.TempDir())
		if !errors.Is(err, consumercontract.ErrRefused) || !strings.Contains(err.Error(), "v1.1.0") {
			t.Fatalf("%s: got %v", tag, err)
		}
	}
	plan, err := consumercontract.Prepare(t.Context(), dir, remote, "v1.1.0", t.TempDir())
	if err != nil || plan.Changed {
		t.Fatalf("the baseline itself: %+v %v", plan, err)
	}
}

func TestBumpToAReleaseWithoutAContractDeclaresNothing(t *testing.T) {
	t.Parallel()
	remote := published(t)
	dir := consumer(t, remote, "v1.2.0", "alpha = x\n")
	state := t.TempDir()
	plan, err := consumercontract.Prepare(t.Context(), dir, remote, "v1.0.0", state)
	if err != nil || plan.Changed || len(plan.Actions) != 0 {
		t.Fatalf("%+v %v", plan, err)
	}
	if changed, applyErr := consumercontract.Apply(dir, state); applyErr != nil || changed {
		t.Fatalf("apply: %v %v", changed, applyErr)
	}
}

func TestBumpRefusesATagTheRemoteDoesNotPublish(t *testing.T) {
	t.Parallel()
	remote := published(t)
	dir := consumer(t, remote, "v1.2.0", "alpha = x\n")
	for _, tag := range []string{"v9.9.9", "main", "../v1.2.0"} {
		if _, err := consumercontract.Prepare(
			t.Context(),
			dir,
			remote,
			tag,
			t.TempDir(),
		); !errors.Is(
			err,
			consumercontract.ErrNoTag,
		) {
			t.Errorf("%s: got %v", tag, err)
		}
	}
}

func TestVerifyRejectsAConfEitherReaderRefuses(t *testing.T) {
	t.Parallel()
	remote := published(t)
	dir := consumer(t, remote, "v1.2.0", "alpha = x\n")
	if err := consumercontract.Verify(t.Context(), dir, t.TempDir()); !errors.Is(err, consumercontract.ErrRefused) {
		t.Fatalf("no prepared bump: %v", err)
	}
	state := t.TempDir()
	if _, err := consumercontract.Prepare(t.Context(), dir, remote, "v1.3.0", state); err != nil {
		t.Fatal(err)
	}
	// The bump was prepared but never applied: the target reader requires gamma.
	if err := consumercontract.Verify(
		t.Context(),
		dir,
		state,
	); !errors.Is(err, consumercontract.ErrRefused) ||
		!strings.Contains(err.Error(), "target") {
		t.Fatalf("unapplied: %v", err)
	}
	if _, err := consumercontract.Apply(dir, state); err != nil {
		t.Fatal(err)
	}
	// Someone then adds a setting that neither reader recognizes.
	writeFile(
		t,
		filepath.Join(dir, filepath.FromSlash(consumercontract.ConfFile)),
		"alpha = x\ngamma = on\nmystery = 1\n",
	)
	if err := consumercontract.Verify(t.Context(), dir, state); !errors.Is(err, consumercontract.ErrRefused) {
		t.Fatalf("unknown key: %v", err)
	}
}

func TestApplyWithoutAPlanChangesNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if changed, err := consumercontract.Apply(dir, t.TempDir()); err != nil || changed {
		t.Fatalf("%v %v", changed, err)
	}
}
