// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Controlled go executable for fuzz orchestration fixtures.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	trace, err := os.OpenFile(os.Getenv("FUZZ_TRACE"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	if err := json.NewEncoder(trace).Encode(os.Args[1:]); err != nil {
		panic(err)
	}
	if err := trace.Close(); err != nil {
		panic(err)
	}
	scenario := os.Getenv("FUZZ_SCENARIO")
	if os.Args[1] == "list" {
		if strings.HasPrefix(scenario, "packages-") {
			fmt.Fprintln(os.Stderr, "package discovery failed")
			if scenario == "packages-partial" {
				fmt.Println("example/one")
			}
			os.Exit(9)
		}
		if scenario == "single" {
			fmt.Println("example/one")
			return
		}
		if scenario != "empty-packages" {
			fmt.Println("example/one\nexample/two")
		}
		return
	}
	if os.Args[2] == "-list" {
		fmt.Fprintln(os.Stderr, "target discovery diagnostic")
		if scenario == "empty-targets" || scenario == "targets-empty" {
			os.Exit(8)
		}
		fmt.Println("TestOrdinary\nFuzzFirst")
		if scenario == "targets-partial" {
			os.Exit(8)
		}
		if scenario != "single" && os.Args[4] == "example/one" {
			fmt.Println("FuzzSecond")
		}
		fmt.Println("ok example")
		return
	}
	fmt.Println("target stdout")
	fmt.Fprintln(os.Stderr, "target stderr")
	if scenario == "target-failure" {
		os.Exit(23)
	}
}
