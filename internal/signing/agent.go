// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package signing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tbhb-dev/devtools/internal/credentials"
)

// Agent uses a clock supplied by the caller so renewal has no background state.
type Agent struct {
	Config Config
	Now    func() time.Time
}

// Environment replaces only the child process's signing socket.
func Environment(socket string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "SSH_AUTH_SOCK=") {
			env = append(env, entry)
		}
	}
	return append(env, "SSH_AUTH_SOCK="+socket)
}

func (a Agent) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = Environment(filepath.Join(a.Config.RunDir, "agent.sock"))
	return cmd
}

// Probe asks the agent to sign through the configured public key, without a disk message.
func (a Agent) Probe(ctx context.Context) error {
	cmd := a.command(ctx, "ssh-keygen", "-Y", "sign", "-n", "devtools-probe", "-U", "-f", a.Config.PublicKey)
	cmd.Stdin = strings.NewReader("probe")
	return commandResult(cmd.Run())
}

func (a Agent) start(ctx context.Context) error {
	cmd := a.command(ctx, "ssh-add", "-l")
	err := cmd.Run()
	if err == nil {
		return nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return nil
	}
	socket := filepath.Join(a.Config.RunDir, "agent.sock")
	if err = os.Remove(socket); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	return commandResult(a.command(ctx, "ssh-agent", "-a", socket).Run())
}

func (a Agent) load(ctx context.Context) error {
	ttl := strconv.FormatInt(int64(a.Config.TTL/time.Second), 10)
	if a.Config.Provider == "file" {
		return commandResult(a.command(ctx, "ssh-add", "-t", ttl, a.Config.KeyFile).Run())
	}
	provider, err := credentials.OPCommand(ctx, a.Config.OPReference, a.Config.TokenCommand, false)
	if err != nil {
		return fmt.Errorf("signing provider: %w", err)
	}
	pipe, err := provider.StdoutPipe()
	if err != nil {
		return fmt.Errorf("provider pipe: %w", err)
	}
	defer pipe.Close()
	add := a.command(ctx, "ssh-add", "-t", ttl, "-")
	add.Stdin = pipe
	if err = provider.Start(); err != nil {
		return fmt.Errorf("start provider: %w", err)
	}
	addErr := add.Run()
	// Close the reader before waiting if ssh-add stops reading early.
	_ = pipe.Close()
	providerErr := provider.Wait()
	if providerErr != nil {
		return fmt.Errorf("read provider: %w", providerErr)
	}
	return commandResult(addErr)
}

func (a Agent) stale() bool {
	data, err := os.ReadFile(filepath.Join(a.Config.RunDir, "key-loaded-at"))
	if err != nil {
		return true
	}
	stamp, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return true
	}
	age := a.Now().Sub(time.Unix(stamp, 0))
	return age < 0 || age >= a.Config.Reload
}

// Ensure serializes start, probe and renewal across independent signer processes.
// A crashed owner leaves the lock in place. Callers time out and use the session agent.
func (a Agent) Ensure(ctx context.Context) error {
	if err := os.MkdirAll(a.Config.RunDir, 0o700); err != nil {
		return fmt.Errorf("create signing directory: %w", err)
	}
	//nolint:gosec // This path is a private directory and needs owner traversal.
	if err := os.Chmod(a.Config.RunDir, 0o700); err != nil {
		return fmt.Errorf("protect signing directory: %w", err)
	}
	lock := filepath.Join(a.Config.RunDir, "lock")
	if err := acquire(ctx, lock); err != nil {
		return err
	}
	defer os.Remove(lock)
	if err := a.start(ctx); err != nil {
		return fmt.Errorf("start signing agent: %w", err)
	}
	if a.Probe(ctx) == nil && !a.stale() {
		return nil
	}
	loadErr := a.load(ctx)
	if loadErr == nil {
		stamp := []byte(strconv.FormatInt(a.Now().Unix(), 10))
		if err := os.WriteFile(filepath.Join(a.Config.RunDir, "key-loaded-at"), stamp, 0o600); err != nil {
			return fmt.Errorf("stamp signing key: %w", err)
		}
	}
	if err := a.Probe(ctx); err != nil {
		return fmt.Errorf("signing probe: %w", errors.Join(err, loadErr))
	}
	return nil
}

func acquire(ctx context.Context, lock string) error {
	const lockTimeout = 10 * time.Second
	const lockPoll = 20 * time.Millisecond
	timer := time.NewTimer(lockTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(lockPoll)
	defer ticker.Stop()
	for {
		err := os.Mkdir(lock, 0o700)
		if err == nil {
			return nil
		}
		if !os.IsExist(err) {
			return fmt.Errorf("lock signing agent: %w", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for signing lock: %w", ctx.Err())
		case <-timer.C:
			return fmt.Errorf("%w: signing lock timed out", os.ErrDeadlineExceeded)
		case <-ticker.C:
		}
	}
}

// Sign forwards Git's ssh-keygen arguments and falls back to the original session agent.
func (a Agent) Sign(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) error {
	err := a.Ensure(ctx)
	if a.Config.Hook != "" {
		status := "ready"
		if err != nil {
			status = "held"
		}
		//nolint:gosec // The optional executable is selected by the operator.
		hook := exec.CommandContext(ctx, a.Config.Hook, status)
		hook.Stderr = stderr
		if hookErr := hook.Run(); hookErr != nil {
			fmt.Fprintln(stderr, "signing status hook:", hookErr)
		}
	}
	cmd := exec.CommandContext(ctx, "ssh-keygen", args...)
	if err == nil {
		cmd.Env = Environment(filepath.Join(a.Config.RunDir, "agent.sock"))
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, stderr
	return commandResult(cmd.Run())
}

func commandResult(err error) error {
	if err != nil {
		return fmt.Errorf("OpenSSH command: %w", err)
	}
	return nil
}
