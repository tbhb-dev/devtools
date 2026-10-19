// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package commitlint

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

const exitInputError = 2

type runOptions struct {
	signoff             string
	currentIdentities   bool
	prePush             string
	requireAssistedFrom string
	rules               string
	file                string
}

// Run is the command entry point. Invalid input or configuration exits 2;
// rule violations exit 1, and successful checks exit 0.
func Run(args []string, in io.Reader, errOut io.Writer, dir string, getenv func(string) string) int {
	options, files, status := parseRunOptions(args, errOut)
	if status >= 0 {
		return status
	}
	config, err := LoadConfig(dir, getenv)
	if err != nil {
		fmt.Fprintf(errOut, "commit-lint: %v\n", err)
		return exitInputError
	}
	if options.signoff != "" {
		return checkSignoffCommand(config, options.signoff, files, errOut)
	}
	if options.prePush != "" {
		if files != 0 || options.currentIdentities || options.requireAssistedFrom != "" {
			fmt.Fprintln(errOut, "commit-lint: --pre-push cannot be combined with a message file or identity context")
			return exitInputError
		}
		return checkPrePush(config, options.rules, options.prePush, in, errOut, dir)
	}
	return checkMessageCommand(config, options, files, in, errOut, dir)
}

func parseRunOptions(args []string, errOut io.Writer) (runOptions, int, int) {
	flags := flag.NewFlagSet("commit-lint", flag.ContinueOnError)
	flags.SetOutput(errOut)
	var options runOptions
	flags.StringVar(&options.signoff, "check-signoff", "", "check one Name <email> sign-off identity")
	flags.BoolVar(&options.currentIdentities, "current-identities", false, "check the identities of the pending commit")
	flags.StringVar(&options.prePush, "pre-push", "", "lint commits from pre-push stdin for this remote")
	flags.StringVar(
		&options.requireAssistedFrom,
		"require-assisted-from",
		"",
		"require attribution when a revision range contains agent commits",
	)
	flags.StringVar(&options.rules, "rules", rulesAll, "rules to check: all, shape, trailers")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options, 0, 0
		}
		return options, 0, exitInputError
	}
	if flags.NArg() > 1 || (options.rules != rulesAll && options.rules != "shape" && options.rules != "trailers") {
		fmt.Fprintln(errOut, "commit-lint: expected --rules all|shape|trailers and at most one message file")
		return options, 0, exitInputError
	}
	if flags.NArg() == 1 {
		options.file = flags.Arg(0)
	}
	return options, flags.NArg(), -1
}

func checkMessageCommand(
	config Config,
	options runOptions,
	files int,
	in io.Reader,
	errOut io.Writer,
	dir string,
) int {
	identities, err := identityContext(dir, options.currentIdentities, options.requireAssistedFrom)
	if err != nil {
		fmt.Fprintf(errOut, "commit-lint: %v\n", err)
		return exitInputError
	}
	var data []byte
	if files == 1 {
		data, err = os.ReadFile(options.file)
	} else {
		data, err = io.ReadAll(in)
	}
	if err != nil {
		fmt.Fprintf(errOut, "commit-lint: %v\n", err)
		return exitInputError
	}
	findings := CheckWithIdentities(Parse(string(data)), config, options.rules, identities)
	for _, finding := range findings {
		fmt.Fprintln(errOut, finding)
	}
	if len(findings) > 0 {
		return 1
	}
	return 0
}

func checkSignoffCommand(config Config, value string, files int, errOut io.Writer) int {
	if files != 0 {
		fmt.Fprintln(errOut, "commit-lint: --check-signoff takes no message file")
		return exitInputError
	}
	if err := config.CheckSignoff(value); err != nil {
		fmt.Fprintf(errOut, "commit-lint: %v\n", err)
		return 1
	}
	return 0
}
