// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package releaseassets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	testTag    = "v1.2.3"
	testCommit = "0123456789012345678901234567890123456789"
	testDate   = "2026-10-26T12:00:00Z"
)

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func testInventory(t *testing.T) inventory {
	t.Helper()
	var inv inventory
	requireNoError(t, json.Unmarshal(inventoryJSON, &inv))
	return inv
}

func buildFixture(t *testing.T) (string, metadata) {
	t.Helper()
	dir := t.TempDir()
	requireNoError(t, build(t.Context(), fakeBuild, testTag, dir))
	return dir, metadata{testTag, testCommit, testDate, testInventory(t)}
}

func fakeBuild(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
	switch name {
	case "git":
		if args[0] == "show" {
			return []byte(testDate), nil
		}
		return []byte(testCommit), nil
	case "bash":
		return []byte("-s -w -buildid="), nil
	case "go":
		out := slices.Index(args, "-o")
		return nil, wrapClose(os.WriteFile(args[out+1], []byte("executable fixture"), 0o600))
	default:
		return nil, errUsage
	}
}

func TestBundles(t *testing.T) {
	t.Parallel()
	dir, meta := buildFixture(t)
	requireNoError(t, verify(dir, meta))
	// The archive interfaces are explicit, including root names and .exe.
	if !slices.Equal(
		meta.Inventory.Platforms,
		[]string{"linux/amd64", "linux/arm64", "darwin/arm64", "windows/amd64"},
	) ||
		!slices.Equal(
			meta.Inventory.Binaries,
			[]string{
				"agentcontext",
				"agenthooks",
				"agentrepo",
				"agentstore",
				"commit-lint",
				"gh-post",
				"gh-reviewer",
				"guard-markdown",
				"ssh-sign",
			},
		) {
		t.Fatal(meta.Inventory)
	}
	for _, platform := range meta.Inventory.Platforms {
		if platform == "windows/amd64" && executable("agentrepo", platform) != "agentrepo.exe" {
			t.Fatal(platform)
		}
		requireNoError(t, verifyArchive(dir, platform, meta))
	}
	second := t.TempDir()
	requireNoError(t, build(t.Context(), fakeBuild, testTag, second))
	for _, name := range append(assetNames(meta), checksumName) {
		first, err := os.ReadFile(filepath.Join(dir, name))
		requireNoError(t, err)
		again, err := os.ReadFile(filepath.Join(second, name))
		requireNoError(t, err)
		if string(first) != string(again) {
			t.Fatalf("non-reproducible %s", name)
		}
	}
}

func TestBuildFailures(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"HEAD", "tag", "date", "flags", "build"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			failure := errEvidence
			runner := func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
				selected := buildStage(name, args) == stage
				if selected {
					return []byte("partial output"), failure
				}
				return fakeBuild(ctx, env, name, args...)
			}
			if err := build(t.Context(), runner, testTag, t.TempDir()); !errors.Is(err, failure) {
				t.Fatalf("got %v", err)
			}
		})
	}
	t.Run("tag differs", func(t *testing.T) {
		t.Parallel()
		runner := func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			if name == "git" && strings.Contains(args[len(args)-1], "refs/tags/") {
				return []byte("different"), nil
			}
			return fakeBuild(ctx, env, name, args...)
		}
		if err := build(t.Context(), runner, testTag, t.TempDir()); !errors.Is(err, errEvidence) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestNegativeEvidence(t *testing.T) {
	t.Parallel()
	for _, mutation := range []string{"missing archive", "missing metadata", "missing checksums", "corrupted bytes", "incorrect checksum", "version", "commit", "date", "platform", "binary", "omitted binary", "extra binary", "unknown metadata"} {
		t.Run(mutation, func(t *testing.T) {
			t.Parallel()
			dir, meta := buildFixture(t)
			mutateEvidence(t, mutation, dir, meta)
			if err := verify(dir, meta); err == nil {
				t.Fatal("accepted invalid evidence")
			}
		})
	}
}

func TestPublish(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"new", "matching", "conflict", "upload failure", "download failure", "list failure", "create failure"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			dir, meta := buildFixture(t)
			stub := &publisherStub{t: t, scenario: scenario, dir: dir, meta: meta}
			runner := stub.command
			err := publish(t.Context(), runner, testTag, dir)
			if scenario == "new" || scenario == "matching" {
				requireNoError(t, err)
			} else if err == nil {
				t.Fatal("accepted failed publication")
			}
			if scenario == "conflict" && stub.writes != 0 {
				t.Fatal("uploaded before comparing existing evidence")
			}
		})
	}
}

func buildStage(name string, args []string) string {
	switch name {
	case "bash":
		return "flags"
	case "go":
		return "build"
	case "git":
		if args[0] == "show" {
			return "date"
		}
		if args[1] == "HEAD" {
			return "HEAD"
		}
		return "tag"
	default:
		return ""
	}
}

func mutateEvidence(t *testing.T, mutation, dir string, meta metadata) {
	t.Helper()
	bad := meta
	bad.Inventory = inventory{slices.Clone(meta.Inventory.Platforms), slices.Clone(meta.Inventory.Binaries)}
	archive := filepath.Join(dir, archiveName(testTag, "linux/amd64"))
	switch mutation {
	case "missing archive":
		requireNoError(t, os.Remove(archive))
	case "missing metadata":
		requireNoError(t, os.Remove(filepath.Join(dir, metadataName)))
	case "missing checksums":
		requireNoError(t, os.Remove(filepath.Join(dir, checksumName)))
	case "corrupted bytes":
		requireNoError(t, os.WriteFile(archive, []byte("bad"), 0o600))
	case "incorrect checksum":
		requireNoError(t, os.WriteFile(filepath.Join(dir, checksumName), []byte("bad"), 0o600))
	case "version":
		bad.Version = "v0.0.0"
	case "commit":
		bad.Commit = strings.Repeat("f", 40)
	case "date":
		bad.BuildDate = "2025-01-01T00:00:00Z"
	case "platform":
		bad.Inventory.Platforms = bad.Inventory.Platforms[:1]
	case "binary":
		bad.Inventory.Binaries = bad.Inventory.Binaries[:1]
	case "unknown metadata":
		requireNoError(t, os.WriteFile(filepath.Join(dir, metadataName), []byte(`{"unknown":true}`), 0o600))
	case "omitted binary", "extra binary":
		mutateArchive(t, mutation, dir, meta)
	}
	if slices.Contains([]string{"version", "commit", "date", "platform", "binary"}, mutation) {
		raw, err := json.Marshal(bad)
		requireNoError(t, err)
		requireNoError(t, os.WriteFile(filepath.Join(dir, metadataName), raw, 0o600))
	}
}

type publisherStub struct {
	t        *testing.T
	scenario string
	dir      string
	meta     metadata
	writes   int
}

func (stub *publisherStub) command(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	if name == "git" {
		return fakeBuild(ctx, env, name, args...)
	}
	switch {
	case name == "gh-post":
		if stub.scenario == "create failure" {
			return nil, errEvidence
		}
		return nil, nil
	case args[0] == "repo":
		return []byte("owner/repo"), nil
	case args[1] == "list":
		if stub.scenario == "list failure" {
			return nil, errEvidence
		}
		if stub.scenario == "new" || stub.scenario == "create failure" {
			return []byte(`[]`), nil
		}
		return []byte(`[{"tagName":"v1.2.3"}]`), nil
	case args[1] == "view":
		return stub.view()
	case args[1] == "upload":
		stub.writes++
		if stub.scenario == "upload failure" {
			return nil, errEvidence
		}
		return nil, nil
	case args[1] == "download":
		if stub.scenario == "download failure" {
			return nil, errEvidence
		}
		return stub.download(args)
	default:
		stub.t.Fatalf("unexpected %s %v", name, args)
		return nil, errUsage
	}
}

func (stub *publisherStub) download(args []string) ([]byte, error) {
	destination, rootErr := os.OpenRoot(args[slices.Index(args, "--dir")+1])
	if rootErr != nil {
		return nil, fmt.Errorf("open fixture root: %w", rootErr)
	}
	defer destination.Close()
	names := append(assetNames(stub.meta), checksumName)
	if index := slices.Index(args, "--pattern"); index >= 0 {
		names = []string{args[index+1]}
	}
	for _, asset := range names {
		data, err := os.ReadFile(filepath.Join(stub.dir, asset))
		if err != nil {
			return nil, fmt.Errorf("release assets: %w", err)
		}
		if stub.scenario == "conflict" {
			data = []byte("different bytes")
		}
		if err = destination.WriteFile(asset, data, 0o600); err != nil {
			return nil, fmt.Errorf("release assets: %w", err)
		}
	}
	return nil, nil
}

func mutateArchive(t *testing.T, mutation, dir string, meta metadata) {
	t.Helper()
	bad := meta
	bad.Inventory.Binaries = slices.Clone(meta.Inventory.Binaries)

	staging := t.TempDir()
	if mutation == "omitted binary" {
		bad.Inventory.Binaries = bad.Inventory.Binaries[:1]
	} else {
		bad.Inventory.Binaries = append(bad.Inventory.Binaries, "unexpected")
	}
	for _, bin := range bad.Inventory.Binaries {
		requireNoError(t, os.WriteFile(filepath.Join(staging, bin), []byte("executable"), 0o600))
	}
	requireNoError(t, pack(dir, staging, "linux/amd64", bad))
	sums, err := checksums(dir, meta)
	requireNoError(t, err)
	requireNoError(t, os.WriteFile(filepath.Join(dir, checksumName), sums, 0o600))
}

func TestRunAndExecute(t *testing.T) {
	t.Parallel()
	dir, meta := buildFixture(t)
	inv := filepath.Join(t.TempDir(), "inventory.json")
	requireNoError(t, os.WriteFile(inv, inventoryJSON, 0o600))
	requireNoError(t, Run(t.Context(), []string{"verify", meta.Version, meta.Commit, meta.BuildDate, dir, inv}))
	for _, args := range [][]string{nil, {"unknown", "tag", "dir"}, {"verify", meta.Version, meta.Commit, meta.BuildDate, dir, "missing"}} {
		if err := Run(t.Context(), args); err == nil {
			t.Fatal("accepted invalid invocation")
		}
	}
	requireNoError(t, os.WriteFile(inv, []byte("{"), 0o600))
	if err := Run(t.Context(), []string{"verify", meta.Version, meta.Commit, meta.BuildDate, dir, inv}); err == nil {
		t.Fatal("accepted malformed inventory")
	}
	out, err := execute(t.Context(), []string{"GOOS=linux"}, "go", "env", "GOOS")
	requireNoError(t, err)
	if strings.TrimSpace(string(out)) != "linux" {
		t.Fatalf("environment was not passed: %s", out)
	}
	if _, err = execute(t.Context(), nil, "devtools-nonexistent-executable"); err == nil {
		t.Fatal("accepted failed command")
	}
}

func (stub *publisherStub) view() ([]byte, error) {
	if stub.scenario == "new" {
		return []byte(`{"tagName":"v1.2.3","assets":[]}`), nil
	}
	return []byte(`{"tagName":"v1.2.3","assets":[{"name":"release.json"}]}`), nil
}

func TestArchiveFailures(t *testing.T) {
	t.Parallel()
	for _, platform := range []string{"linux/amd64", "windows/amd64"} {
		t.Run(platform, func(t *testing.T) {
			t.Parallel()
			dir, meta := buildFixture(t)
			missing := meta
			missing.Inventory.Binaries = missing.Inventory.Binaries[:1]
			staging := t.TempDir()
			requireNoError(
				t,
				os.WriteFile(
					filepath.Join(staging, executable(missing.Inventory.Binaries[0], platform)),
					[]byte("fixture"),
					0o600,
				),
			)
			requireNoError(t, pack(dir, staging, platform, missing))
			if err := verifyArchive(dir, platform, meta); err == nil {
				t.Fatal("accepted omitted binary")
			}
			requireNoError(
				t,
				os.WriteFile(filepath.Join(dir, archiveName(testTag, platform)), []byte("corrupt archive"), 0o600),
			)
			if err := verifyArchive(dir, platform, meta); err == nil {
				t.Fatal("accepted corrupt archive")
			}
		})
	}
}

func TestCompleteRetry(t *testing.T) {
	t.Parallel()
	dir, meta := buildFixture(t)
	stub := &publisherStub{t: t, scenario: "matching", dir: dir, meta: meta}
	existing := release{Tag: testTag}
	for _, name := range append(assetNames(meta), checksumName) {
		existing.Assets = append(existing.Assets, struct {
			Name string `json:"name"`
		}{name})
	}
	requireNoError(t, upload(t.Context(), stub.command, "owner/repo", dir, meta, existing))
	if stub.writes != 0 {
		t.Fatal("replaced matching assets")
	}
}

func TestTrailingMetadata(t *testing.T) {
	t.Parallel()
	dir, meta := buildFixture(t)
	raw, err := os.ReadFile(filepath.Join(dir, metadataName))
	requireNoError(t, err)
	root, rootErr := os.OpenRoot(dir)
	requireNoError(t, rootErr)
	defer root.Close()
	requireNoError(t, root.WriteFile(metadataName, append(raw, []byte("{}")...), 0o600))
	if err = verify(dir, meta); err == nil {
		t.Fatal("accepted trailing JSON")
	}
}

func TestBadBuildInputs(t *testing.T) {
	t.Parallel()
	for _, platform := range []string{"linux/amd64", "windows/amd64"} {
		t.Run(platform, func(t *testing.T) {
			t.Parallel()
			_, meta := buildFixture(t)
			if err := pack(t.TempDir(), t.TempDir(), platform, meta); err == nil {
				t.Fatal("packed missing binaries")
			}
		})
	}
	runner := func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
		if name == "git" && args[0] == "show" {
			return []byte("invalid date"), nil
		}
		return fakeBuild(ctx, env, name, args...)
	}
	if err := build(t.Context(), runner, testTag, t.TempDir()); err == nil {
		t.Fatal("accepted invalid commit date")
	}
	blocked := filepath.Join(t.TempDir(), "file")
	requireNoError(t, os.WriteFile(blocked, []byte("not a directory"), 0o600))
	if err := build(t.Context(), fakeBuild, testTag, blocked); err == nil {
		t.Fatal("accepted invalid output directory")
	}
}

func TestPublisherReadFailures(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"repo", "view", "list"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			dir, meta := buildFixture(t)
			stub := &publisherStub{t: t, scenario: "matching", dir: dir, meta: meta}
			runner := func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
				if name == "gh" && (args[0] == operation || args[1] == operation) {
					return failedRead(operation)
				}
				return stub.command(ctx, env, name, args...)
			}
			if err := publish(t.Context(), runner, testTag, dir); err == nil {
				t.Fatal("accepted bad release read")
			}
			if stub.writes != 0 {
				t.Fatal("uploaded after bad release read")
			}
		})
	}
}

func failedRead(operation string) ([]byte, error) {
	if operation == "repo" {
		return []byte("partial"), errEvidence
	}
	return []byte("invalid json"), nil
}
