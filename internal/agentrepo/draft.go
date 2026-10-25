// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	draftControl  = "This is a very robust and comprehensive design that does not use contractions and it is significantly better.\n"
	draftUsage    = 2
	draftNotFound = 127
)

func draftCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:                "lint-draft <file>",
		Short:              "Run every prose gate over one drafted document",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			*code = lintDraft(cmd.Context(), args, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func draftExec(ctx context.Context, root, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "LC_ALL=C", "PYTHONUTF8=1")
	return cmd
}

func draftStatus(err error) int {
	if err == nil {
		return 0
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode()
	}
	return draftNotFound
}

func lintDraft(ctx context.Context, args []string, out, stderr io.Writer) int {
	git := draftExec(ctx, "", "git", "rev-parse", "--show-toplevel")
	git.Stderr = stderr
	data, err := git.Output()
	if err != nil {
		return 1
	}
	root := strings.TrimSuffix(string(data), "\n")
	if len(args) == 0 || args[0] == "" {
		_, _ = fmt.Fprintln(stderr, "usage: lint-draft.sh <file>")
		return draftUsage
	}
	file := args[0]
	if status := draftTarget(root, file, out, stderr); status != 0 {
		return status
	}
	status := draftProbe(ctx, root, file, out)
	gates := []*exec.Cmd{
		draftExec(ctx, root, "vale", "--output=ai-tells-agent.tmpl", file),
		draftExec(ctx, root, "cspell", "--config", ".cspell.jsonc", "--no-summary", "--no-progress", file),
	}
	if strings.HasSuffix(file, ".md") {
		gates = append(gates, draftExec(ctx, root, "rumdl", "check", file))
	}
	for _, gate := range gates {
		gate.Stdout, gate.Stderr = out, stderr
		if runErr := gate.Run(); runErr != nil {
			if gate.ProcessState == nil {
				_, _ = fmt.Fprintln(stderr, runErr)
			}
			status = 1
		}
	}
	return status
}

func draftTarget(root, file string, out, stderr io.Writer) int {
	path := file
	if !filepath.IsAbs(path) {
		// Keep parent components until symlinks resolve.
		path = root + string(filepath.Separator) + path
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		_, _ = fmt.Fprintf(out, "%s:1 [error] missing-file  no such file\n", file)
		return 1
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		_, _ = fmt.Fprintf(
			out,
			"%s:1 [error] outside-repo  cspell only lints files under the repository; move the draft inside it\n",
			file,
		)
		return 1
	}
	switch filepath.Ext(file) {
	case ".go", ".py", ".sh":
		_, _ = fmt.Fprintf(out, "%s:1 [error] wrong-task  source file; use mise run lint-prose\n", file)
		return draftUsage
	}
	return 0
}

func draftProbe(ctx context.Context, root, file string, out io.Writer) int {
	probe := draftExec(ctx, root, "vale", "--path="+file, "--output=ai-tells-agent.tmpl")
	probe.Stdin = strings.NewReader(draftControl)
	data, err := probe.CombinedOutput()
	probeText := strings.TrimRight(string(data), "\n")
	if rc := draftStatus(err); rc > 1 {
		if len(data) == 0 && err != nil {
			probeText = err.Error()
		}
		_, _ = fmt.Fprintf(
			out,
			"%s:1 [error] vale-failed  vale exited %d instead of linting, so this run measured nothing: %s\n",
			file,
			rc,
			strings.ReplaceAll(probeText, "\n", " "),
		)
		return 1
	} else if !draftFindings(probeText) {
		_, _ = fmt.Fprintf(
			out,
			"%s:1 [error] unscoped-path  no .vale.ini section matches this path, so vale loads no styles and a silent run proves nothing; move the draft to a path the config names\n",
			file,
		)
		return 1
	}
	return 0
}

func draftFindings(text string) bool {
	for line := range strings.SplitSeq(text, "\n") {
		if len(line) > 0 && line[0] >= '0' && line[0] <= '9' {
			return true
		}
	}
	return false
}
