// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoTag reports a repository without a release tag to compare against.
var ErrNoTag = errors.New("no release tag")

const (
	// maxExtracted bounds one extracted file, since a tag's tree is untrusted.
	maxExtracted = 4 << 20
	// ownerExecute is the owner's execute bit in an archive entry's mode.
	ownerExecute = 0o100
	// plainFileMode and executableFileMode are the modes extracted files get.
	plainFileMode      os.FileMode = 0o600
	executableFileMode os.FileMode = 0o700
)

// PreviousTag names the newest release tag reachable from rev.
func PreviousTag(ctx context.Context, repo, rev string) (string, error) {
	out, err := git(ctx, repo, hermeticEnv(os.TempDir()), "describe", "--tags", "--abbrev=0", "--match", "v[0-9]*", rev)
	if err != nil {
		return "", fmt.Errorf("%w: reachable from %s", ErrNoTag, rev)
	}
	return strings.TrimSpace(out), nil
}

// ReadAt reads a file at a revision. It reports false when the file isn't
// there, and an error only when Git can't answer.
func ReadAt(ctx context.Context, repo, rev, path string) ([]byte, bool, error) {
	env := hermeticEnv(os.TempDir())
	res, err := run(ctx, repo, env, "", "git", "cat-file", "-e", rev+":"+path)
	if err != nil || !res.ok {
		return nil, false, err
	}
	out, err := git(ctx, repo, env, "cat-file", "blob", rev+":"+path)
	if err != nil {
		return nil, false, err
	}
	return []byte(out), true, nil
}

// Extract writes the readers and contract of a revision into the destination
// directory: the scripts under each skills directory and the contract file. A
// revision without some of them still extracts the rest.
func Extract(ctx context.Context, repo, rev, dest string) error {
	env := hermeticEnv(os.TempDir())
	dirs := skillDirs()
	specs := make([]string, 0, 2*len(dirs)+1)
	for _, dir := range dirs {
		specs = append(specs, dir+"/"+commitScripts, dir+"/"+mergeScripts)
	}
	specs = append(specs, ContractPath)
	listed, err := git(ctx, repo, env, append([]string{"ls-tree", "-r", "--name-only", "-z", rev, "--"}, specs...)...)
	if err != nil {
		return err
	}
	files := strings.Split(strings.TrimRight(listed, "\x00"), "\x00")
	if len(files) == 1 && files[0] == "" {
		return nil
	}
	archive, err := git(ctx, repo, env, append([]string{"archive", "--format=tar", rev, "--"}, files...)...)
	if err != nil {
		return err
	}
	return untar(bytes.NewReader([]byte(archive)), dest)
}

func untar(in io.Reader, dest string) error {
	reader := tar.NewReader(in)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(header.Name))
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(filepath.Separator)) {
			return fmt.Errorf("%w: archive path %q leaves the destination", ErrCommand, header.Name)
		}
		data, err := io.ReadAll(io.LimitReader(reader, maxExtracted))
		if err != nil {
			return fmt.Errorf("read %s: %w", header.Name, err)
		}
		if err = writeExtracted(target, data, header.Mode&ownerExecute != 0); err != nil {
			return err
		}
	}
}

// writeExtracted writes one file from an archive and preserves the owner's
// execute bit. The extracted scripts still run.
func writeExtracted(target string, data []byte, executable bool) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
	}
	mode := plainFileMode
	if executable {
		mode = executableFileMode
	}
	if err := os.WriteFile(target, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}

// FetchTag fetches one release tag from a remote into a scratch repository
// under dir and returns its path. Only the tag's own commit is fetched.
func FetchTag(ctx context.Context, remote, tag, dir string) (string, error) {
	if !tagForm.MatchString(tag) {
		return "", fmt.Errorf("%w: %q is not a release tag", ErrNoTag, tag)
	}
	repo := filepath.Join(dir, "tag-"+tag)
	env := hermeticEnv(dir)
	if err := os.MkdirAll(repo, 0o750); err != nil {
		return "", fmt.Errorf("create %s: %w", repo, err)
	}
	if _, err := git(ctx, repo, env, "init", "-q"); err != nil {
		return "", err
	}
	if _, err := git(
		ctx,
		repo,
		env,
		"fetch",
		"-q",
		"--depth",
		"1",
		"--no-tags",
		remote,
		"refs/tags/"+tag+":refs/tags/"+tag,
	); err != nil {
		return "", fmt.Errorf("%w: %s is not published at %s", ErrNoTag, tag, remote)
	}
	return repo, nil
}
