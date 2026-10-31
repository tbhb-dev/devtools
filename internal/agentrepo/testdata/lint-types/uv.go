// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// This fixture records invocation details and emits controlled stream writes.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	record := struct {
		Args      []string `json:"Args"`
		Locale    string   `json:"Locale"`
		Directory string   `json:"Directory"`
		Unrelated string   `json:"Unrelated"`
	}{os.Args[1:], os.Getenv("LC_ALL"), cwd, os.Getenv("LINT_TYPES_UNRELATED")}
	trace, err := os.OpenFile("lint-types-trace", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	if err = json.NewEncoder(trace).Encode(record); err != nil {
		panic(err)
	}
	if err = trace.Close(); err != nil {
		panic(err)
	}
	var chunks []struct {
		Stream string `json:"Stream"`
		Text   string `json:"Text"`
	}
	if err = json.Unmarshal([]byte(os.Getenv("LINT_TYPES_CHUNKS")), &chunks); err != nil {
		panic(err)
	}
	for _, chunk := range chunks {
		stream := os.Stdout
		if chunk.Stream == "err" {
			stream = os.Stderr
		}
		if _, err = fmt.Fprint(stream, chunk.Text); err != nil {
			panic(err)
		}
	}
	code, err := strconv.Atoi(os.Getenv("LINT_TYPES_STATUS"))
	if err != nil {
		panic(err)
	}
	os.Exit(code)
}
