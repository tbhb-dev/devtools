// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package commitlint

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Identity matches an email pattern and, when supplied, an exact name.
// Only * and ? are wildcards. Brackets in GitHub bot addresses are literal.
type Identity struct {
	Email string `toml:"email"`
	Name  string `toml:"name"`
}

// Identities restricts sign-offs. Agents is reusable for author attribution rules.
type Identities struct {
	Agents  []Identity `toml:"agents"`
	SignOff []Identity `toml:"sign_off"`
}

var (
	errInvalidSignoff  = errors.New("invalid sign-off identity; use the responsible person's Name <email>")
	errAgentSignoff    = errors.New("denied agent sign-off; the responsible person must sign off")
	errUnlistedSignoff = errors.New("sign-off is not allowed; a configured responsible person must sign off")
)

var identityPattern = regexp.MustCompile(`^([^<>]+) <([^<>[:space:]]+)>$`)

func (i Identity) matches(name, email string) bool {
	pattern := regexp.QuoteMeta(strings.ToLower(i.Email))
	pattern = strings.ReplaceAll(pattern, `\*`, `.*`)
	pattern = strings.ReplaceAll(pattern, `\?`, `.`)
	return (i.Name == "" || i.Name == name) && regexp.MustCompile("^"+pattern+"$").MatchString(strings.ToLower(email))
}

// CheckSignoff applies restrictions even when sign-off presence is optional.
func (c Config) CheckSignoff(value string) error {
	if len(c.Identities.Agents) == 0 && len(c.Identities.SignOff) == 0 {
		return nil
	}
	match := identityPattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return fmt.Errorf("%w: %q", errInvalidSignoff, value)
	}
	name := strings.TrimSpace(match[1])
	for _, agent := range c.Identities.Agents {
		if agent.matches(name, match[2]) {
			return fmt.Errorf("%w: %q", errAgentSignoff, value)
		}
	}
	if len(c.Identities.SignOff) > 0 {
		for _, allowed := range c.Identities.SignOff {
			if allowed.matches(name, match[2]) {
				return nil
			}
		}
		return fmt.Errorf("%w: %q", errUnlistedSignoff, value)
	}
	return nil
}
