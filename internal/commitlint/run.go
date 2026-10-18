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

// Run is the command entry point. Invalid input or configuration exits 2;
// rule violations exit 1, and successful checks exit 0.
func Run(args []string, in io.Reader, errOut io.Writer, dir string, getenv func(string) string) int {
	flags := flag.NewFlagSet("commit-lint", flag.ContinueOnError)
	flags.SetOutput(errOut)
	signoff := flags.String("check-signoff", "", "check one Name <email> sign-off identity")
	rules := flags.String("rules", rulesAll, "rules to check: all, shape, trailers")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return exitInputError
	}
	if flags.NArg() > 1 || (*rules != rulesAll && *rules != "shape" && *rules != "trailers") {
		fmt.Fprintln(errOut, "commit-lint: expected --rules all|shape|trailers and at most one message file")
		return exitInputError
	}
	config, err := LoadConfig(dir, getenv)
	if err != nil {
		fmt.Fprintf(errOut, "commit-lint: %v\n", err)
		return exitInputError
	}
	if *signoff != "" {
		return checkSignoffCommand(config, *signoff, flags.NArg(), errOut)
	}
	var data []byte
	if flags.NArg() == 1 {
		data, err = os.ReadFile(flags.Arg(0))
	} else {
		data, err = io.ReadAll(in)
	}
	if err != nil {
		fmt.Fprintf(errOut, "commit-lint: %v\n", err)
		return exitInputError
	}
	findings := Check(Parse(string(data)), config, *rules)
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
