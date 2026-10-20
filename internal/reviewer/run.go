// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package reviewer

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tbhb-dev/devtools/internal/credentials"
	"github.com/tbhb-dev/devtools/internal/ghapp"
)

// Runner exposes the clock, GitHub transport and final exec for offline tests.
type Runner struct {
	Config Config
	Client ghapp.Client
	Now    func() time.Time
	Exec   func(string, []string, []string) error
}

// Run handles explicit installation or replaces this process with GitHub CLI.
func Run(ctx context.Context, home string, args []string) error {
	if len(args) > 0 && args[0] == "--install" {
		const forceArgCount = 2
		force := len(args) == forceArgCount && args[1] == "--force"
		if len(args) != 1 && !force {
			return fmt.Errorf("%w: use --install [--force]", os.ErrInvalid)
		}
		binary, err := os.Executable()
		if err != nil {
			return fmt.Errorf("binary path: %w", err)
		}
		return Install(home, binary, force)
	}
	config, err := ReadConfig(home, os.Getenv)
	if err != nil {
		return err
	}
	const requestTimeout = 30 * time.Second
	runner := Runner{
		Config: config,
		Client: ghapp.Client{HTTP: &http.Client{Timeout: requestTimeout}, BaseURL: "https://api.github.com"},
		Now:    time.Now,
		Exec:   execGH,
	}
	return runner.Run(ctx, args)
}

// Run obtains a cached or fresh token, then preserves CLI arguments and exit status.
func (r Runner) Run(ctx context.Context, args []string) error {
	repo, err := Repository(ctx, args)
	if err != nil {
		return err
	}
	token, ok := cachedToken(r.Config, repo, r.Now())
	if !ok {
		token, err = r.mint(ctx, repo)
		if err != nil {
			return err
		}
		if err = storeToken(r.Config, repo, token); err != nil {
			return err
		}
	}
	binary, err := exec.LookPath("gh")
	if err != nil {
		return fmt.Errorf("gh path: %w", err)
	}
	env := credentials.ReplaceEnv(os.Environ(), "GH_TOKEN", token.Value)
	// The child needs only the installation token, never the provider's service account.
	env = credentials.ReplaceEnv(env, "OP_SERVICE_ACCOUNT_TOKEN", "")
	if err = r.Exec(binary, append([]string{"gh"}, args...), env); err != nil {
		return fmt.Errorf("exec gh: %w", err)
	}
	return nil
}

func (r Runner) mint(ctx context.Context, repo string) (ghapp.Token, error) {
	appID := r.Config.AppID
	if r.Config.Provider == "op" {
		data, err := credentials.Read(ctx, "op", r.Config.AppIDReference, r.Config.TokenCommand)
		if err != nil {
			return ghapp.Token{}, fmt.Errorf("reviewer credential: %w", err)
		}
		appID = strings.TrimSpace(string(data))
	}
	key, err := credentials.Read(ctx, r.Config.Provider, r.Config.KeySource, r.Config.TokenCommand)
	if err != nil {
		return ghapp.Token{}, fmt.Errorf("reviewer credential: %w", err)
	}
	jwt, err := ghapp.JWT(appID, key, r.Now())
	if err != nil {
		return ghapp.Token{}, fmt.Errorf("reviewer credential: %w", err)
	}
	owner, name, _ := strings.Cut(repo, "/")
	token, err := r.Client.Mint(ctx, jwt, owner, name, r.Config.Installation)
	if err != nil {
		return ghapp.Token{}, fmt.Errorf("reviewer token: %w", err)
	}
	return token, nil
}
