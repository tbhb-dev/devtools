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

// publishedLater extends published with a release that adds nothing to
// what a conf needs and declares none, so the key v1.3.0 began requiring is
// only in the history below it.
func publishedLater(t *testing.T) string {
	t.Helper()
	requireProbeTools(t)
	return releases(t,
		release{"v1.0.0", "reader-v1", ""},
		release{"v1.1.0", "reader-v1", "contract-baseline.toml"},
		release{"v1.2.0", "reader-v2", "contract-bridge.toml"},
		release{"v1.3.0", "reader-v3", "contract-require.toml"},
		release{"v1.4.0", "reader-v3", "contract-after-require.toml"},
	)
}

// AC10: a target that declares none still refuses a consumer that never
// adopted what an earlier release required, and the refusal gives the bridge
// from that release's declaration rather than a generic reason.
func TestBumpNamesTheBridgeWhenALaterTargetDeclaresNone(t *testing.T) {
	t.Parallel()
	remote := publishedLater(t)
	dir := consumer(t, remote, "v1.1.0", "alpha = x\n")
	_, err := consumercontract.Prepare(t.Context(), dir, remote, "v1.4.0", t.TempDir())
	if !errors.Is(err, consumercontract.ErrRefused) || !strings.Contains(err.Error(), "update to v1.2.0 first") {
		t.Fatalf("got %v", err)
	}
	if got := confOf(t, dir); got != "alpha = x\n" {
		t.Fatalf("a refused bump edited the conf: %q", got)
	}
}

// AC10: a consumer past the bridge is sent to the release holding the
// transition, not back to a release it already has.
func TestBumpNamesTheRequiringReleaseForAConsumerPastTheBridge(t *testing.T) {
	t.Parallel()
	remote := publishedLater(t)
	dir := consumer(t, remote, "v1.2.0", "alpha = x\n")
	_, err := consumercontract.Prepare(t.Context(), dir, remote, "v1.4.0", t.TempDir())
	if !errors.Is(err, consumercontract.ErrRefused) || !strings.Contains(err.Error(), "update to v1.3.0 first") {
		t.Fatalf("got %v", err)
	}
}

// AC5: nothing to edit. The consumer already meets everything the earlier releases asked of a conf. A no-transition target leaves the file alone.
func TestBumpToALaterNoneTargetLeavesASatisfiedConsumerAlone(t *testing.T) {
	t.Parallel()
	remote := publishedLater(t)
	dir := consumer(t, remote, "v1.3.0", "alpha = x\ngamma = on\n")
	state := t.TempDir()
	plan, err := consumercontract.Prepare(t.Context(), dir, remote, "v1.4.0", state)
	if err != nil || plan.Changed || len(plan.Actions) != 0 {
		t.Fatalf("%+v %v", plan, err)
	}
}

// AC10: a conf the consumer's own reader already rejects is refused before
// any edit, even when the target declares nothing to apply.
func TestBumpRefusesAConfTheSourceReaderRejectsEvenWithNoTransition(t *testing.T) {
	t.Parallel()
	remote := publishedLater(t)
	dir := consumer(t, remote, "v1.3.0", "alpha = x\ngamma = on\nmystery = 1\n")
	state := t.TempDir()
	_, err := consumercontract.Prepare(t.Context(), dir, remote, "v1.4.0", state)
	if !errors.Is(err, consumercontract.ErrRefused) || !strings.Contains(err.Error(), "as it stands") {
		t.Fatalf("got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(state, "plan.conf")); statErr == nil {
		t.Fatal("a refused bump left a plan to apply")
	}
	if changed, applyErr := consumercontract.Apply(dir, state); applyErr != nil || changed {
		t.Fatalf("apply after a refusal: %v %v", changed, applyErr)
	}
}

// AC10: same refusal for a tag from before the contract.
func TestBumpToAReleaseWithoutAContractStillJudgesTheConf(t *testing.T) {
	t.Parallel()
	remote := published(t)
	dir := consumer(t, remote, "v1.2.0", "alpha = x\nmystery = 1\n")
	if _, err := consumercontract.Prepare(
		t.Context(),
		dir,
		remote,
		"v1.0.0",
		t.TempDir(),
	); !errors.Is(
		err,
		consumercontract.ErrRefused,
	) {
		t.Fatalf("got %v", err)
	}
}
