// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// metadataOutput keeps Git's formatting independent of the caller's locale.
func metadataOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	data, err := cmd.Output()
	return strings.TrimRight(string(data), "\n"), err
}

func metadataGit(ctx context.Context, fallback string, args ...string) string {
	value, err := metadataOutput(ctx, "git", args...)
	if err != nil {
		return fallback
	}
	return value
}

func goLdflags(ctx context.Context) (string, error) {
	module := os.Getenv("DEVTOOLS_GO_MODULE")
	if module == "" {
		var err error
		module, err = metadataOutput(ctx, "go", "list", "-m")
		if err != nil {
			return "", fmt.Errorf("go list -m: %w", err)
		}
	}
	pkg := os.Getenv("DEVTOOLS_GO_BUILDMETA")
	if pkg == "" {
		pkg = module + "/internal/buildmeta"
	}
	commit := metadataGit(ctx, "", "rev-parse", "--short=7", "HEAD")
	version := os.Getenv("DEVTOOLS_GO_VERSION")
	if version == "" {
		fallback := metadataGit(ctx, "DEV", "rev-parse", "--short=7", "HEAD")
		version = metadataGit(ctx, fallback, "describe", "--tags", "--abbrev=7")
	}
	date := metadataGit(
		ctx,
		"unknown",
		"-c",
		"log.showSignature=false",
		"log",
		"-1",
		"--format=%cd",
		"--date=format-local:%Y-%m-%dT%H:%M:%SZ",
	)
	return fmt.Sprintf(
		"-s -w -buildid= -X %s.Version=%s -X %s.Commit=%s -X %s.Date=%s\n",
		pkg,
		version,
		pkg,
		commit,
		pkg,
		date,
	), nil
}

func ldflagsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "go-ldflags",
		Short: "Print reproducible Go linker flags; DEVTOOLS_GO_VERSION optionally fixes Version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := goLdflags(cmd.Context())
			if err != nil {
				return err
			}
			if _, err = fmt.Fprint(cmd.OutOrStdout(), flags); err != nil {
				return fmt.Errorf("write Go linker flags: %w", err)
			}
			return nil
		},
	}
}
