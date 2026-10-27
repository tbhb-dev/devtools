// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/buildmeta"
	"github.com/tbhb-dev/devtools/internal/shellallow"
)

// defaultAllowlist is the allow list's path, relative to the working
// directory.
const defaultAllowlist = ".devtools/shell-allowlist.toml"

// Exit statuses. A script that breaks the allow list isn't a command that
// failed, so findings and errors get distinct codes.
const (
	exitOK       = 0
	exitFindings = 1
	exitError    = 2
)

// errFindings signals that the check reported commands. Run maps it to
// exitFindings without printing it, which leaves the reports as the only
// output.
var errFindings = errors.New("findings")

// streams gathers the process output so tests can drive the command without
// touching the real stdout and stderr.
type streams struct {
	out io.Writer
	err io.Writer
}

// Run executes the command and returns the process exit status.
func Run(args []string, s streams) int {
	cmd := newRootCmd(s)
	cmd.SetArgs(args)

	switch err := cmd.Execute(); {
	case err == nil:
		return exitOK
	case errors.Is(err, errFindings):
		return exitFindings
	default:
		fmt.Fprintln(s.err, "shell-allowlist:", err)
		return exitError
	}
}

func newRootCmd(s streams) *cobra.Command {
	var allowlist string

	root := &cobra.Command{
		Use:   "shell-allowlist [flags] files...",
		Short: "Report shell commands that are not on the allow list",
		Long: "Parses each named file as shell and reports every command that the\n" +
			"allow list doesn't permit, with the file, the line, and the command.\n" +
			"The interpreters and data tools in the banned list are reported\n" +
			"whatever the allow list says, and a command word that expands at run\n" +
			"time is reported as unresolvable.\n\n" +
			"Exactly the named files are checked, whatever their names. The command\n" +
			"neither walks directories nor reads shebangs. It prints nothing and\n" +
			"exits 0 when every command is allowed, and exits 1 after reporting.",
		Version:       buildmeta.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, files []string) error {
			return check(allowlist, files, s)
		},
	}

	root.Flags().StringVar(&allowlist, "allowlist", defaultAllowlist,
		"path of the TOML allow list")
	root.SetOut(s.out)
	root.SetErr(s.err)
	return root
}

// check reports every disallowed command in files.
func check(allowlist string, files []string, s streams) error {
	list, err := shellallow.Load(allowlist)
	if err != nil {
		return fmt.Errorf("allow list %s: %w", allowlist, err)
	}
	found := false
	for _, file := range files {
		// The path is the caller's own argument, and the command exists to
		// read it.
		src, openErr := os.Open(file)
		if openErr != nil {
			return fmt.Errorf("opening %s: %w", file, openErr)
		}
		findings, checkErr := list.Check(file, src)
		_ = src.Close()
		if checkErr != nil {
			return fmt.Errorf("checking %s: %w", file, checkErr)
		}
		for _, finding := range findings {
			found = true
			fmt.Fprintln(s.out, finding)
		}
	}
	if found {
		return errFindings
	}
	return nil
}
