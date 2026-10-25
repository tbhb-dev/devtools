// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

const markerUsage = "usage: marker-scan.sh [--staged] <path> [path ...]"

type markerScanner struct {
	attributes func([]string) ([]byte, error)
	read       func(string, bool) ([]byte, error)
}

func markersCommand(code *int) *cobra.Command {
	group := &cobra.Command{Use: "markers", Short: "Find leftover conflict markers"}
	group.AddCommand(&cobra.Command{
		Use:                "scan [--staged] <path> [path ...]",
		Short:              "Report marker line counts in argument order",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			staged := len(args) > 0 && args[0] == "--staged"
			if staged {
				args = args[1:]
			}
			if len(args) == 0 {
				*code = 2
				cmd.PrintErrln(markerUsage)
				return nil
			}
			scanner := markerScanner{
				attributes: func(paths []string) ([]byte, error) {
					git := exec.CommandContext(cmd.Context(), "git", "check-attr", "-z", "--stdin", "conflict-markers")
					git.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
					return git.Output()
				},
				read: func(path string, staged bool) ([]byte, error) {
					return readMarkerFile(cmd.Context(), path, staged)
				},
			}
			output := scanner.scan(args, staged)
			if output != "" {
				*code = 1
			}
			_, err := fmt.Fprint(cmd.OutOrStdout(), output)
			if err != nil {
				return fmt.Errorf("write marker report: %w", err)
			}
			return nil
		},
	})
	return group
}

func readMarkerFile(ctx context.Context, path string, staged bool) ([]byte, error) {
	if staged {
		git := exec.CommandContext(ctx, "git", "cat-file", "blob")
		git.Args = append(git.Args, ":"+path)
		data, err := git.Output()
		if err != nil {
			return nil, fmt.Errorf("read index marker path: %w", err)
		}
		return data, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat marker path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read marker path: %w", err)
	}
	return data, nil
}

func (s markerScanner) scan(paths []string, staged bool) string {
	candidates := make([]string, 0, len(paths))
	for _, path := range paths {
		if path != "" && !markerSkill(path) {
			candidates = append(candidates, path)
		}
	}
	exempt := s.documented(candidates)
	var output strings.Builder
	for _, path := range candidates {
		if exempt[path] {
			continue
		}
		data, err := s.read(path, staged)
		if err != nil {
			continue
		}
		if count := markerLines(data); count > 0 {
			fmt.Fprintf(&output, "%s  %d marker line(s)\n", path, count)
		}
	}
	return output.String()
}

func (s markerScanner) documented(paths []string) map[string]bool {
	exempt := make(map[string]bool)
	if len(paths) == 0 {
		return exempt
	}
	data, err := s.attributes(paths)
	// A failed lookup mustn't trust even valid output printed before failure.
	if err != nil {
		return exempt
	}
	fields := strings.Split(string(data), "\x00")
	const tuple = 3
	for i := 0; i+tuple < len(fields); i += tuple {
		if fields[i+1] == "conflict-markers" && fields[i+2] == "documented" {
			exempt[fields[i]] = true
		}
	}
	return exempt
}

func markerSkill(path string) bool {
	const skill = "skills/resolve-rebase-conflicts/SKILL.md"
	return path == skill || strings.HasSuffix(path, "/"+skill)
}

func markerLines(data []byte) int {
	count := 0
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		const width = 7
		if len(line) < width || (len(line) > width && line[width] != ' ') {
			continue
		}
		switch string(line[:width]) {
		case "<<<<<<<", ">>>>>>>", "|||||||":
			count++
		}
	}
	return count
}
