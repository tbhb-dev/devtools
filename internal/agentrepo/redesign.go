// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/config"
)

var (
	errRedesignKind    = errors.New("kind must be commit, pr or squash")
	errRedesignReason  = errors.New("a nonempty --reason is required")
	errRedesignMissing = errors.New("no active review record")
	errRedesignFile    = errors.New("review record must be a regular file; inspect it")
	errRedesignBound   = errors.New("review_max_rounds must be an integer from 1 to 9")
	errRedesignLegacy  = errors.New("legacy review record has no rejected digests; inspect it")
	errRedesignSecond  = errors.New("a second redesign requires inspection")
	errRedesignRecord  = errors.New("invalid review record or changed bound; inspect it")
	errRedesignDigest  = errors.New("invalid rejected digest; inspect the review record")
	errRedesignEarly   = errors.New("review record is not at the finding bound")
)

func redesignCommand(code *int) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "redesign <kind> --reason TEXT",
		Short: "Start a logged redesign after the finding bound",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := startRedesign(cmd, args[0], reason); err != nil {
				*code = 3
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "BLOCKED:", err)
				return
			}
			cmd.Println("Redesign started. Rewrite the draft before reviewing again.")
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for rewriting the draft (required)")
	return cmd
}

func startRedesign(cmd *cobra.Command, kind, reason string) error {
	if !slices.Contains([]string{"commit", "pr", "squash"}, kind) {
		return errRedesignKind
	}
	if strings.TrimSpace(reason) == "" {
		return errRedesignReason
	}
	dir, err := configGit(cmd, "--absolute-git-dir")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "agent-skills", kind+".rounds")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return errRedesignMissing
	}
	if err != nil {
		return fmt.Errorf("review record: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errRedesignFile
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read review record: %w", err)
	}
	lookup := config.Lookup{RepositoryRoot: func() (string, error) { return configGit(cmd, "--show-toplevel") }}
	value, err := lookup.Repository("review_max_rounds", "3")
	if err != nil {
		return fmt.Errorf("review bound: %w", err)
	}
	bound, err := strconv.Atoi(value)
	if err != nil || bound < 1 || bound > 9 {
		return errRedesignBound
	}
	event, err := redesignEvent(data, kind, reason, bound)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return fmt.Errorf("open review record: %w", err)
	}
	defer file.Close()
	if _, err = file.Write(event); err != nil {
		return fmt.Errorf("append redesign: %w", err)
	}
	return nil
}

func redesignEvent(data []byte, kind, reason string, bound int) ([]byte, error) {
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[1], "bound ") {
		return nil, errRedesignLegacy
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "redesign ") {
			return nil, errRedesignSecond
		}
	}
	subject := strings.TrimPrefix(lines[0], "subject ")
	if subject == "" || subject == lines[0] || lines[1] != "bound "+strconv.Itoa(bound) ||
		!strings.HasSuffix(string(data), "\n") {
		return nil, errRedesignRecord
	}
	rejected := make([]string, 0, len(lines))
	for _, line := range lines[2:] {
		digest := strings.TrimPrefix(line, "finding ")
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size || digest == line {
			return nil, errRedesignDigest
		}
		rejected = append(rejected, digest)
	}
	if len(rejected) != bound {
		return nil, errRedesignEarly
	}
	event := struct {
		Kind     string   `json:"kind"`
		Subject  string   `json:"subject"`
		At       string   `json:"at"`
		Reason   string   `json:"reason"`
		Bound    int      `json:"bound"`
		Rejected []string `json:"rejected"`
	}{kind, subject, time.Now().UTC().Format(time.RFC3339), reason, bound, rejected}
	encoded, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("encode redesign: %w", err)
	}
	return append(append([]byte("redesign "), encoded...), '\n'), nil
}
