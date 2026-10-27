// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func lintSourceCommand(code *int) *cobra.Command {
	return &cobra.Command{
		Use:                "lint-source-files MODE [ARG ...]",
		Short:              "Select owned source files and invoke their lint gate",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && args[0] == "--help" {
				return cmd.Help()
			}
			if len(args) == 0 {
				cmd.PrintErrln("lint-source-files: mode required")
				*code = 1
				return nil
			}
			*code = lintSource(cmd.Context(), args[0], args[1:], cmd.OutOrStdout(), cmd.ErrOrStderr(), nil)
			return nil
		},
	}
}

func lintGate(mode string) (string, []string) {
	const shellPattern = "*.sh"
	switch mode {
	case "shell":
		return shellPattern, []string{"shellcheck", "-o", "check-extra-masked-returns"}
	case "curl":
		return shellPattern, []string{"uv", "run", "--script", lintScript("check_curl_fail.py")}
	case "shell-fmt":
		return shellPattern, []string{"shfmt", "-d"}
	case "prose":
		glob := os.Getenv("DEVTOOLS_PROSE_GLOB")
		if glob == "" {
			glob = "*"
		}
		return "*", []string{"vale", "--output=ai-tells-agent.tmpl", "--glob=" + glob}
	case "markdown-wrap":
		return "*.md", []string{"guard-markdown"}
	default:
		return "", nil
	}
}

func lintScript(name string) string { return os.Getenv("DEVTOOLS_DIR") + "/scripts/" + name }

func lintDiscovery(patterns []string) []string {
	args := []string{"ls-files", "-z", "--cached", "--others", "--exclude-standard", "--"}
	args = append(args, patterns...)
	args = append(args, ":(exclude)vendor/**", ":(exclude)apm_modules/**")
	for path := range strings.SplitSeq(os.Getenv("DEVTOOLS_GENERATED_PATHS"), "\n") {
		if path != "" {
			path = strings.TrimSuffix(path, "/")
			args = append(args, ":(exclude)"+path, ":(exclude)"+path+"/**")
		}
	}
	return args
}

// lintSource keeps Git selection semantics and delegates exact deployment filtering
// to the existing Python helper. An operational failure never retries selection.
func lintSource(ctx context.Context, mode string, args []string, stdout, stderr io.Writer,
	execute func(string, []string, io.Writer) int,
) int {
	pattern, gate := lintGate(mode)
	if gate == nil {
		_, _ = fmt.Fprintln(stderr, "lint-source-files: unsupported mode:", mode)
		return 1
	}
	if execute == nil {
		execute = func(name string, argv []string, out io.Writer) int { return executeLint(ctx, name, argv, out, stderr) }
	}
	patterns := []string{pattern}
	if mode == "prose" && len(args) > 0 {
		patterns, args = args, nil
	}
	selected, err := os.CreateTemp("", "devtools-lint-*")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer os.Remove(selected.Name())
	status := execute("git", lintDiscovery(patterns), selected)
	closeErr := selected.Close()
	if status != 0 {
		return status
	}
	if closeErr != nil {
		_, _ = fmt.Fprintln(stderr, closeErr)
		return 1
	}
	filterArgs := []string{"run", "--script", lintScript("filter-apm-deployed.py"), selected.Name()}
	if status = execute("uv", filterArgs, stdout); status != 0 {
		return status
	}
	data, err := os.ReadFile(selected.Name())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	files := bytes.Split(data, []byte{0})
	if len(files) == 1 {
		return 0
	}
	gate = append(gate, args...)
	// The shell read loop accepts only records ending in a zero byte.
	for _, file := range files[:len(files)-1] {
		gate = append(gate, string(file))
	}
	return execute(gate[0], gate[1:], stdout)
}

func executeLint(ctx context.Context, name string, args []string, stdout, stderr io.Writer) int {
	// Executable names come only from the supported modes and discovery steps.
	var command *exec.Cmd
	switch name {
	case "git":
		command = exec.CommandContext(ctx, "git", args...)
	case "uv":
		command = exec.CommandContext(ctx, "uv", args...)
	case "shellcheck":
		command = exec.CommandContext(ctx, "shellcheck", args...)
	case "shfmt":
		command = exec.CommandContext(ctx, "shfmt", args...)
	case "vale":
		command = exec.CommandContext(ctx, "vale", args...)
	case "guard-markdown":
		command = exec.CommandContext(ctx, "guard-markdown", args...)
	}
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, stdout, stderr
	err := command.Run()
	if err == nil {
		return 0
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode()
	}
	_, _ = fmt.Fprintln(stderr, err)
	const notFound, notExecutable = 127, 126
	if errors.Is(err, exec.ErrNotFound) {
		return notFound
	}
	return notExecutable
}
