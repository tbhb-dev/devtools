// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/tbhb-dev/devtools/internal/prsize"
)

var errSizeDraft = errors.New("size: draft requires YAML frontmatter with base")

var errSizeLimit = errors.New(
	"pull request exceeds 800 code SLOC or 2000 prose lines; split by concern, along commit boundaries, or as a stacked series",
)

func checkSizeLimit(out io.Writer, r prsize.Result, login string, bot bool) error {
	_, _ = fmt.Fprintf(out, "%d code SLOC, %d prose lines (limits: 800 / 2000)\n", r.Code, r.Prose)
	if bot && (login == "app/renovate" || login == "renovate[bot]") {
		_, _ = fmt.Fprintln(out, "Renovate-authored pull request exempt")
		return nil
	}
	if r.Code > 800 || r.Prose > 2000 {
		return errSizeLimit
	}
	return nil
}

func sizeCheckCommand() *cobra.Command {
	var base, head, draft string
	var publish bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Refuse pull requests above the hard size limits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			publishBase, err := sizePublishBase(cmd, draft, base)
			if err != nil {
				return err
			}
			var login string
			var bot bool
			if publish {
				login, bot, err = sizeAuthor(cmd)
				if err != nil {
					return err
				}
			}
			r, err := prsize.Measure(cmd.Context(), "", publishBase, head)
			if err != nil {
				return fmt.Errorf("size: %w", err)
			}
			if err = checkSizeLimit(cmd.OutOrStdout(), r, login, bot); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Publish base (origin ref preferred)")
	cmd.Flags().StringVar(&head, "head", "HEAD", "Head ref")
	cmd.Flags().StringVar(&draft, "draft", "", "Read publish base from draft when present")
	cmd.Flags().BoolVar(&publish, "publish", false, "Look up the existing pull request author")
	return cmd
}

func sizePublishBase(cmd *cobra.Command, draft, base string) (string, error) {
	if draft != "" {
		var err error
		base, err = sizeDraftBase(draft, base)
		if err != nil {
			return "", err
		}
	}
	if base != "" {
		remote := "refs/remotes/origin/" + base
		resolve := exec.CommandContext(cmd.Context(), "git")
		resolve.Args = append(resolve.Args, "rev-parse", "--verify", "--quiet", "--end-of-options", remote+"^{commit}")
		if resolve.Run() == nil {
			base = remote
		}
	}
	return base, nil
}

func sizeAuthor(cmd *cobra.Command) (string, bool, error) {
	branch, err := exec.CommandContext(cmd.Context(), "git", "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		return "", false, fmt.Errorf("size branch: %w", err)
	}
	query := exec.CommandContext(cmd.Context(), "gh")
	query.Args = append(
		query.Args,
		"pr",
		"list",
		"--head",
		strings.TrimSpace(string(branch)),
		"--state",
		"open",
		"--json",
		"author",
	)
	query.Stderr = cmd.ErrOrStderr()
	data, err := query.Output()
	if err != nil {
		return "", false, fmt.Errorf("size: read pull request author: %w", err)
	}
	var prs []struct {
		Author struct {
			Login string `json:"login"`
			IsBot bool   `json:"is_bot"`
		} `json:"author"`
	}
	if err = json.Unmarshal(data, &prs); err != nil {
		return "", false, fmt.Errorf("size: author JSON: %w", err)
	}
	if len(prs) == 0 {
		return "", false, nil
	}
	return prs[0].Author.Login, prs[0].Author.IsBot, nil
}

func sizeDraftBase(draft, fallback string) (string, error) {
	data, err := os.ReadFile(draft)
	if os.IsNotExist(err) {
		return fallback, nil
	}
	if err != nil {
		return "", fmt.Errorf("size draft: %w", err)
	}
	const frontmatterParts = 3
	parts := bytes.SplitN(data, []byte("---"), frontmatterParts)
	if len(parts) != frontmatterParts || len(bytes.TrimSpace(parts[0])) != 0 {
		return "", errSizeDraft
	}
	var properties struct {
		Base string `yaml:"base"`
	}
	if err = yaml.Unmarshal(parts[1], &properties); err != nil {
		return "", fmt.Errorf("size draft: %w", err)
	}
	if properties.Base == "" {
		return "", errSizeDraft
	}
	return properties.Base, nil
}
