// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Command gates records the subprocess contract for draft linting.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	name := filepath.Base(os.Args[0])
	input := ""
	if strings.HasPrefix(os.Args[1], "--path=") {
		name = "probe"
		data, _ := io.ReadAll(os.Stdin)
		input = string(data)
	}
	cwd, _ := os.Getwd()
	log, err := os.OpenFile(os.Getenv("DRAFT_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	_ = json.NewEncoder(log).Encode(struct {
		Name, Input, Cwd, Locale, UTF8 string
		Args                           []string
	}{name, input, cwd, os.Getenv("LC_ALL"), os.Getenv("PYTHONUTF8"), os.Args[1:]})
	_ = log.Close()
	key := "DRAFT_" + strings.ToUpper(name)
	_, _ = fmt.Fprint(os.Stdout, os.Getenv(key+"_OUT"))
	_, _ = fmt.Fprint(os.Stderr, os.Getenv(key+"_ERR"))
	code, _ := strconv.Atoi(os.Getenv(key + "_CODE"))
	os.Exit(code)
}
