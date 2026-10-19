// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package commitlint

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const (
	prePushFieldCount  = 4
	identityTailFields = 2
	commitFieldCount   = 5
	identityPairSize   = 2
)

var (
	errMissingRemote        = errors.New("--pre-push requires a remote name")
	errMalformedPrePushLine = errors.New(
		"malformed pre-push line: expected local ref, local sha, remote ref, remote sha",
	)
	errInvalidGitVar     = errors.New("git var returned an invalid identity")
	errInvalidCommitData = errors.New("git show returned invalid commit metadata")
	errGitCommand        = errors.New("git command failed")
)

type gitCommit struct {
	short     string
	subject   string
	author    string
	committer string
	message   string
}

func checkPrePush(config Config, rules, remote string, in io.Reader, errOut io.Writer, dir string) int {
	commits, err := pushedCommits(dir, remote, in)
	if err != nil {
		fmt.Fprintf(errOut, "commit-lint: %v\n", err)
		return exitInputError
	}
	found := false
	for _, commit := range commits {
		findings := CheckWithIdentities(
			Parse(commit.message),
			config,
			rules,
			[]string{commit.author, commit.committer},
		)
		for _, finding := range findings {
			fmt.Fprintf(errOut, "%s %s: %s\n", commit.short, commit.subject, finding)
			found = true
		}
	}
	if found {
		fmt.Fprintln(
			errOut,
			"commit-lint: push blocked; fix each commit with an amend or a reworded rebase through the commit skill, then push again",
		)
		return 1
	}
	return 0
}

func pushedCommits(dir, remote string, in io.Reader) ([]gitCommit, error) {
	if remote == "" {
		return nil, errMissingRemote
	}
	seen := make(map[string]bool)
	var commits []gitCommit
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		local, remoteOID, err := parsePushLine(scanner.Text())
		if err != nil {
			return nil, err
		}
		if allZero(local) {
			continue
		}
		objectIDs, err := outgoingOIDs(dir, remote, local, remoteOID)
		if err != nil {
			return nil, err
		}
		commits, err = appendCommits(dir, commits, seen, objectIDs)
		if err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read pre-push input: %w", err)
	}
	return commits, nil
}

func parsePushLine(line string) (string, string, error) {
	fields := strings.Fields(line)
	if len(fields) != prePushFieldCount {
		return "", "", errMalformedPrePushLine
	}
	return fields[1], fields[3], nil
}

func outgoingOIDs(dir, remote, local, remoteOID string) ([]string, error) {
	args := []string{"rev-list", "--reverse", local, "--not"}
	if allZero(remoteOID) {
		args = append(args, "--remotes")
	} else {
		args = append(args, remoteOID, "--remotes="+remote)
	}
	return gitLines(dir, args...)
}

func appendCommits(
	dir string,
	commits []gitCommit,
	seen map[string]bool,
	objectIDs []string,
) ([]gitCommit, error) {
	for _, oid := range objectIDs {
		if seen[oid] {
			continue
		}
		commit, readErr := readCommit(dir, oid)
		if readErr != nil {
			return nil, readErr
		}
		seen[oid] = true
		commits = append(commits, commit)
	}
	return commits, nil
}

func identityContext(dir string, current bool, revision string) ([]string, error) {
	var identities []string
	if current {
		values, err := currentGitIdentities(dir)
		if err != nil {
			return nil, err
		}
		identities = append(identities, values...)
	}
	if revision != "" {
		values, err := revisionIdentities(dir, revision)
		if err != nil {
			return nil, err
		}
		identities = append(identities, values...)
	}
	return identities, nil
}

func currentGitIdentities(dir string) ([]string, error) {
	identities := make([]string, 0, identityPairSize)
	for _, variable := range []string{"GIT_AUTHOR_IDENT", "GIT_COMMITTER_IDENT"} {
		value, err := gitOutput(dir, "var", variable)
		if err != nil {
			return nil, err
		}
		fields := strings.Fields(value)
		if len(fields) <= identityTailFields {
			return nil, fmt.Errorf("%w: %s", errInvalidGitVar, variable)
		}
		identities = append(identities, strings.Join(fields[:len(fields)-identityTailFields], " "))
	}
	return identities, nil
}

func revisionIdentities(dir, revision string) ([]string, error) {
	objectIDs, err := gitLines(dir, "rev-list", revision)
	if err != nil {
		return nil, err
	}
	identities := make([]string, 0, len(objectIDs)*identityPairSize)
	for _, oid := range objectIDs {
		commit, readErr := readCommit(dir, oid)
		if readErr != nil {
			return nil, readErr
		}
		identities = append(identities, commit.author, commit.committer)
	}
	return identities, nil
}

func readCommit(dir, oid string) (gitCommit, error) {
	format := "%h%x00%s%x00%an <%ae>%x00%cn <%ce>%x00%B"
	data, err := gitBytes(dir, "show", "-s", "--format="+format, oid)
	if err != nil {
		return gitCommit{}, err
	}
	parts := bytes.SplitN(data, []byte{0}, commitFieldCount)
	if len(parts) != commitFieldCount {
		return gitCommit{}, fmt.Errorf("%w for %s", errInvalidCommitData, oid)
	}
	return gitCommit{
		short:     string(parts[0]),
		subject:   string(parts[1]),
		author:    string(parts[2]),
		committer: string(parts[3]),
		message:   strings.TrimSuffix(string(parts[4]), "\n"),
	}, nil
}

func gitLines(dir string, args ...string) ([]string, error) {
	output, err := gitOutput(dir, args...)
	if err != nil || output == "" {
		return nil, err
	}
	return strings.Split(output, "\n"), nil
}

func gitOutput(dir string, args ...string) (string, error) {
	data, err := gitBytes(dir, args...)
	return strings.TrimSpace(string(data)), err
}

func gitBytes(dir string, args ...string) ([]byte, error) {
	//nolint:gosec // Git receives an argv vector without a shell.
	command := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf(
			"%w: git %s: %s",
			errGitCommand,
			strings.Join(args, " "),
			strings.TrimSpace(string(output)),
		)
	}
	return output, nil
}

func allZero(value string) bool {
	return value != "" && strings.Trim(value, "0") == ""
}
