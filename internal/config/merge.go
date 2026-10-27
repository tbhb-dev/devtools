// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package config

import (
	"errors"
	"fmt"
	"strings"
)

var errMergeEnvironment = errors.New("merge authority refuses environment variable")

// MergeMachine refuses environment authority and reads only machine files.
func (l Lookup) MergeMachine(key, fallback string) (string, error) {
	for _, authority := range []string{"merge_owner", "required_reviewer", "required_reviewer_type", "delegation_check", "gh_login"} {
		name := "DEVTOOLS_" + strings.ToUpper(authority)
		if _, set := l.LookupEnv(name); set {
			return "", fmt.Errorf("%w %s; use machine configuration", errMergeEnvironment, name)
		}
	}
	return l.machineFile(key, fallback)
}
