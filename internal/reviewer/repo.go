// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package reviewer

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Repository resolves the explicit repo flag, otherwise the checkout origin.
func Repository(ctx context.Context, args []string) (string, error) {
	var target string
	explicit, needsValue := false, false
	for _, arg := range args {
		if needsValue {
			target = arg
			needsValue = false
			continue
		}
		if arg == "--" {
			break
		}
		switch {
		case arg == "-R" || arg == "--repo":
			explicit, needsValue = true, true
		case strings.HasPrefix(arg, "--repo="):
			explicit = true
			target = strings.TrimPrefix(arg, "--repo=")
		case strings.HasPrefix(arg, "-R"):
			explicit = true
			target = strings.TrimPrefix(strings.TrimPrefix(arg, "-R"), "=")
		}
	}
	if needsValue {
		return "", fmt.Errorf("%w: repo flag requires a value", os.ErrInvalid)
	}
	if !explicit {
		output, err := exec.CommandContext(ctx, "git", "remote", "get-url", "origin").Output()
		if err != nil {
			return "", fmt.Errorf("resolve origin: %w", err)
		}
		target = strings.TrimSpace(string(output))
	}
	return parseRepository(target)
}

func parseRepository(target string) (string, error) {
	target = strings.TrimPrefix(target, "git@github.com:")
	if strings.Contains(target, "://") {
		parsed, err := url.Parse(target)
		if err != nil || parsed.Hostname() != "github.com" {
			return "", fmt.Errorf("%w: repository must be on github.com", os.ErrInvalid)
		}
		target = strings.TrimPrefix(parsed.Path, "/")
	}
	target = strings.TrimPrefix(target, "github.com/")
	target = strings.TrimSuffix(target, ".git")
	valid, err := regexp.MatchString(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`, target)
	if err != nil {
		return "", fmt.Errorf("repository pattern: %w", err)
	}
	if !valid {
		return "", fmt.Errorf("%w: expected OWNER/REPO on github.com", os.ErrInvalid)
	}
	for part := range strings.SplitSeq(target, "/") {
		if part == "." || part == ".." {
			return "", fmt.Errorf("%w: invalid repository component", os.ErrInvalid)
		}
	}
	return strings.ToLower(target), nil
}
