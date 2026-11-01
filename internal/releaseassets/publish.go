// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package releaseassets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type release struct {
	Tag    string `json:"tagName"`
	Assets []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

func publish(ctx context.Context, cmd command, tag, dir string) error {
	meta, err := releaseMetadata(ctx, cmd, tag)
	if err != nil {
		return err
	}
	if err = verify(dir, meta); err != nil {
		return err
	}
	repo, err := cmd(ctx, nil, "gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	repository := strings.TrimSpace(string(repo))
	pages, err := cmd(ctx, nil, "gh", "release", "list", "--repo", repository, "--limit", "1000", "--json", "tagName")
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	var releases []release
	if err = json.Unmarshal(pages, &releases); err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	existing := findRelease(releases, tag)
	if existing == nil {
		notes := filepath.Join(dir, "notes.md")
		if err = os.WriteFile(
			notes,
			[]byte("Prebuilt devtools commands. See release.json and SHA256SUMS for build evidence.\n"),
			0o600,
		); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
		_, err = cmd(
			ctx,
			nil,
			"gh-post",
			"release",
			"create",
			tag,
			"--repo",
			repository,
			"--verify-tag",
			"--title",
			tag,
			"--body-file",
			notes,
		)
		if err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
		existing = &release{Tag: tag}
	}
	snapshot, err := cmd(ctx, nil, "gh", "release", "view", tag, "--repo", repository, "--json", "tagName,assets")
	if err != nil {
		return err
	}
	if err = json.Unmarshal(snapshot, existing); err != nil {
		return fmt.Errorf("read release assets: %w", err)
	}

	if existing.Tag != tag {
		return fmt.Errorf("release tag: %w", errEvidence)
	}
	return upload(ctx, cmd, repository, dir, meta, *existing)
}

func upload(ctx context.Context, cmd command, repo, dir string, meta metadata, existing release) error {
	names := append(assetNames(meta), checksumName)
	temp, err := os.MkdirTemp("", "devtools-published-*")
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	defer os.RemoveAll(temp)
	present, err := compareExisting(ctx, cmd, repo, dir, temp, meta, existing)
	if err != nil {
		return err
	}

	for _, name := range names {
		if present[name] {
			continue
		}
		_, err = cmd(ctx, nil, "gh", "release", "upload", meta.Version, filepath.Join(dir, name), "--repo", repo)
		if err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
	}
	// Read back every asset after upload. A successful CLI exit alone isn't evidence.
	downloaded, err := os.MkdirTemp("", "devtools-read-back-*")
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	defer os.RemoveAll(downloaded)
	if _, err = cmd(
		ctx,
		nil,
		"gh",
		"release",
		"download",
		meta.Version,
		"--repo",
		repo,
		"--dir",
		downloaded,
	); err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	for _, name := range names {
		if err = compareFile(dir, downloaded, name); err != nil {
			return err
		}
	}
	return verify(downloaded, meta)
}

func findRelease(releases []release, tag string) *release {
	for _, item := range releases {
		if item.Tag == tag {
			return &item
		}
	}
	return nil
}

func compareExisting(
	ctx context.Context,
	cmd command,
	repo, dir, temp string,
	meta metadata,
	existing release,
) (map[string]bool, error) {
	names := append(assetNames(meta), checksumName)
	present := make(map[string]bool)
	for _, asset := range existing.Assets {
		if present[asset.Name] || !slices.Contains(names, asset.Name) {
			return nil, fmt.Errorf("unexpected or duplicate asset %s: %w", asset.Name, errEvidence)
		}
		present[asset.Name] = true
		if _, err := cmd(
			ctx,
			nil,
			"gh",
			"release",
			"download",
			meta.Version,
			"--repo",
			repo,
			"--pattern",
			asset.Name,
			"--dir",
			temp,
		); err != nil {
			return nil, err
		}
		if err := compareFile(dir, temp, asset.Name); err != nil {
			return nil, err
		}
	}
	return present, nil
}

func compareFile(local, remote, name string) error {
	first, err := os.ReadFile(filepath.Join(local, name))
	if err != nil {
		return fmt.Errorf("read local asset: %w", err)
	}
	second, err := os.ReadFile(filepath.Join(remote, name))
	if err != nil {
		return fmt.Errorf("read published asset: %w", err)
	}
	if !bytes.Equal(first, second) {
		return fmt.Errorf("existing asset %s has different bytes: %w", name, errEvidence)
	}
	return nil
}
