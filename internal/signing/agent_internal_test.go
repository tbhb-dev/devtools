// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package signing

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T) Agent {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("OpenSSH Unix agent sockets")
	}
	for _, tool := range []string{"ssh-agent", "ssh-add", "ssh-keygen"} {
		_, err := exec.LookPath(tool)
		require.NoError(t, err)
	}
	// Short paths fit the Unix socket limit even under long worktree names.
	//nolint:usetesting // Unix sockets need shorter paths than t.TempDir under this checkout.
	dir, err := os.MkdirTemp("", "sign-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	key := filepath.Join(dir, "source-key")
	require.NoError(t, exec.CommandContext(t.Context(), "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).Run())
	a := Agent{
		Config: Config{
			RunDir:    filepath.Join(dir, "run"),
			PublicKey: key + ".pub",
			Provider:  "file",
			KeyFile:   key,
			TTL:       time.Hour,
			Reload:    45 * time.Minute,
		},
		Now: time.Now,
	}
	t.Cleanup(func() {
		// Kill only the temporary agent, never the session's agent.
		cmd := a.command(context.Background(), "ssh-agent", "-k")
		data, readErr := os.ReadFile(filepath.Join(a.Config.RunDir, "pid"))
		if readErr == nil {
			cmd.Env = append(cmd.Env, "SSH_AGENT_PID="+string(data))
			require.NoError(t, cmd.Run())
		}
	})
	return a
}

func startFixture(t *testing.T, a Agent) {
	t.Helper()
	require.NoError(t, os.MkdirAll(a.Config.RunDir, 0o700))
	output, err := a.command(t.Context(), "ssh-agent", "-a", filepath.Join(a.Config.RunDir, "agent.sock")).Output()
	require.NoError(t, err)
	_, tail, ok := strings.Cut(string(output), "SSH_AGENT_PID=")
	require.True(t, ok)
	pid, _, ok := strings.Cut(tail, ";")
	require.True(t, ok)
	require.NoError(t, os.WriteFile(filepath.Join(a.Config.RunDir, "pid"), []byte(pid), 0o600))
}

func fakeOP(t *testing.T, a Agent) string {
	t.Helper()
	dir := filepath.Dir(a.Config.KeyFile)
	key, err := os.ReadFile(a.Config.KeyFile)
	require.NoError(t, err)
	require.NoError(t, os.Remove(a.Config.KeyFile))
	t.Setenv("OP_TEST_KEY", string(key))
	script := "#!/bin/sh\nprintf x >> \"$OP_CALLS\"\nprintf '%s\\n' \"$OP_TEST_KEY\"\n"
	//nolint:gosec // The fake provider must be executable.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "op"), []byte(script), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	calls := filepath.Join(dir, "calls")
	t.Setenv("OP_CALLS", calls)
	return calls
}

//nolint:paralleltest // The fake provider changes the process environment.
func TestRenewalAndProbe(t *testing.T) {
	a := fixture(t)
	startFixture(t, a)
	calls := fakeOP(t, a)
	a.Config.Provider = "op"
	a.Config.OPReference = "test-reference"
	now := time.Now()
	a.Now = func() time.Time { return now }
	require.NoError(t, a.Ensure(t.Context()))
	require.NoError(t, a.Probe(t.Context()))
	now = now.Add(44 * time.Minute)
	require.NoError(t, a.Ensure(t.Context()))
	data, err := os.ReadFile(calls)
	require.NoError(t, err)
	require.Equal(t, "x", string(data))
	now = now.Add(time.Minute)
	require.NoError(t, a.Ensure(t.Context()))
	data, err = os.ReadFile(calls)
	require.NoError(t, err)
	require.Equal(t, "xx", string(data))
	// An empty agent overrides a fresh stamp.
	require.NoError(t, a.command(t.Context(), "ssh-add", "-D").Run())
	require.NoError(t, a.Ensure(t.Context()))
	data, err = os.ReadFile(calls)
	require.NoError(t, err)
	require.Equal(t, "xxx", string(data))
	entries, err := os.ReadDir(a.Config.RunDir)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.Type()&os.ModeSocket != 0 {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(a.Config.RunDir, entry.Name()))
		require.NoError(t, readErr)
		require.NotContains(t, string(content), "PRIVATE KEY")
	}
	require.NoFileExists(t, a.Config.KeyFile)
}

//nolint:paralleltest // The fake provider changes the process environment.
func TestConcurrentSigners(t *testing.T) {
	a := fixture(t)
	startFixture(t, a)
	calls := fakeOP(t, a)
	a.Config.Provider = "op"
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			var out bytes.Buffer
			results <- a.Sign(t.Context(), []string{"-Y", "sign", "-n", "test", "-f", a.Config.PublicKey}, strings.NewReader("message"), &out, &out)
		})
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	data, err := os.ReadFile(calls)
	require.NoError(t, err)
	require.Equal(t, "x", string(data))
}

func TestFallback(t *testing.T) {
	session := fixture(t)
	startFixture(t, session)
	require.NoError(t, session.Ensure(t.Context()))
	socket := filepath.Join(session.Config.RunDir, "agent.sock")
	t.Setenv("SSH_AUTH_SOCK", socket)
	broken := session
	broken.Config.RunDir = filepath.Join(session.Config.KeyFile, "not-a-directory")
	var out, stderr bytes.Buffer
	require.NoError(
		t,
		broken.Sign(
			t.Context(),
			[]string{"-Y", "sign", "-U", "-n", "test", "-f", session.Config.PublicKey},
			strings.NewReader("fallback"),
			&out,
			&stderr,
		),
	)
	require.Contains(t, out.String(), "BEGIN SSH SIGNATURE")
	require.Equal(t, socket, os.Getenv("SSH_AUTH_SOCK"))
	tools := t.TempDir()
	//nolint:gosec // This executable simulates a daemon that refuses to start.
	require.NoError(t, os.WriteFile(filepath.Join(tools, "ssh-agent"), []byte("#!/bin/sh\nexit 70\n"), 0o700))
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	broken.Config.RunDir = filepath.Join(filepath.Dir(session.Config.RunDir), "refused")
	out.Reset()
	require.NoError(
		t,
		broken.Sign(
			t.Context(),
			[]string{"-Y", "sign", "-U", "-n", "test", "-f", session.Config.PublicKey},
			strings.NewReader("fallback"),
			&out,
			&stderr,
		),
	)
	require.Contains(t, out.String(), "BEGIN SSH SIGNATURE")
	require.Equal(t, socket, os.Getenv("SSH_AUTH_SOCK"))
	require.NoFileExists(t, filepath.Join(broken.Config.RunDir, "agent.sock"))
}

func TestStartAndFileProvider(t *testing.T) {
	t.Parallel()
	a := fixture(t)
	// Save the newly started daemon's process ID from a socket query for cleanup.
	require.NoError(t, a.Ensure(t.Context()))
	require.NoError(t, a.Probe(t.Context()))
	// ssh-agent doesn't publish its process ID through the protocol. Find only the
	// process whose exact command line contains the unique temporary socket.
	//nolint:gosec // The unique temporary socket identifies only our test daemon.
	output, err := exec.CommandContext(t.Context(), "pgrep", "-f", "ssh-agent -a "+filepath.Join(a.Config.RunDir, "agent.sock")).
		Output()
	require.NoError(t, err)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(a.Config.RunDir, "pid"), []byte(strings.TrimSpace(string(output))), 0o600),
	)
}

func TestLockAndFailedLoad(t *testing.T) {
	t.Parallel()
	a := fixture(t)
	startFixture(t, a)
	a.Config.KeyFile = "/missing/devtools-test-key"
	require.Error(t, a.Ensure(t.Context()))
	require.NoFileExists(t, filepath.Join(a.Config.RunDir, "key-loaded-at"))
	require.NoError(t, os.Mkdir(filepath.Join(a.Config.RunDir, "lock"), 0o700))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, a.Ensure(ctx), context.Canceled)
}

func TestProviderFailuresAndHook(t *testing.T) {
	a := fixture(t)
	startFixture(t, a)
	fakeOP(t, a)
	a.Config.Provider = "op"
	dir := filepath.Dir(a.Config.KeyFile)
	token := filepath.Join(dir, "token")
	//nolint:gosec // Test executables need owner execute permission.
	require.NoError(t, os.WriteFile(token, []byte("#!/bin/sh\nprintf test-token"), 0o700))
	a.Config.TokenCommand = token
	require.NoError(t, a.Ensure(t.Context()))
	// A failed renewal keeps using a key that still probes successfully.
	a.Now = func() time.Time { return time.Now().Add(time.Hour) }
	a.Config.TokenCommand = filepath.Join(dir, "missing")
	require.NoError(t, a.Ensure(t.Context()))
	hook := filepath.Join(dir, "hook")
	//nolint:gosec // Test executables need owner execute permission.
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\nprintf '%s' \"$1\" > \"$HOOK_RESULT\"\nexit 1\n"), 0o700))
	result := filepath.Join(dir, "hook-result")
	t.Setenv("HOOK_RESULT", result)
	a.Config.Hook = hook
	var out, stderr bytes.Buffer
	args := []string{"-Y", "sign", "-U", "-n", "test", "-f", a.Config.PublicKey}
	require.NoError(t, a.Sign(t.Context(), args, strings.NewReader("message"), &out, &stderr))
	data, err := os.ReadFile(result)
	require.NoError(t, err)
	require.Equal(t, "ready", string(data))
	require.Contains(t, stderr.String(), "signing status hook")
	require.NoError(t, a.command(t.Context(), "ssh-add", "-D").Run())
	require.Error(t, a.Sign(t.Context(), args, strings.NewReader("message"), &out, &stderr))
	data, err = os.ReadFile(result)
	require.NoError(t, err)
	require.Equal(t, "held", string(data))
	// A missing provider fails without publishing a success stamp.
	a.Config.TokenCommand = ""
	t.Setenv("PATH", dir)
	require.Error(t, a.load(t.Context()))
}

func TestAgentLifetime(t *testing.T) {
	t.Parallel()
	a := fixture(t)
	startFixture(t, a)
	require.NoError(t, a.Ensure(t.Context()))
	// Limit the short lifetime to the expiry check.
	a.Config.TTL = time.Second
	require.NoError(t, a.load(t.Context()))
	time.Sleep(1500 * time.Millisecond)
	require.Error(t, a.Probe(t.Context()))
	// The reloaded key must survive runner delays after Ensure returns.
	a.Config.TTL = time.Hour
	require.NoError(t, a.Ensure(t.Context()))
	require.NoError(t, a.Probe(t.Context()))
}

func TestInvalidStamp(t *testing.T) {
	t.Parallel()
	a := fixture(t)
	startFixture(t, a)
	path := filepath.Join(a.Config.RunDir, "key-loaded-at")
	require.NoError(t, os.WriteFile(path, []byte("corrupt"), 0o600))
	require.True(t, a.stale())
	require.NoError(t, os.WriteFile(path, []byte("9999999999"), 0o600))
	require.True(t, a.stale())
}

func TestGitCommit(t *testing.T) {
	t.Parallel()
	a := fixture(t)
	startFixture(t, a)
	require.NoError(t, a.Ensure(t.Context()))
	// The commit must use the agent: keep only the public key on disk.
	require.NoError(t, os.Remove(a.Config.KeyFile))
	binary := filepath.Join(filepath.Dir(a.Config.RunDir), "ssh-sign")
	output, err := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../cmd/ssh-sign").CombinedOutput()
	require.NoError(t, err, string(output))
	repo := t.TempDir()
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "GIT_") || strings.HasPrefix(entry, "DEVTOOLS_SIGNING_") {
			continue
		}
		env = append(env, entry)
	}
	env = append(
		env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=Signing Test",
		"GIT_AUTHOR_EMAIL=signing@example.invalid",
		"GIT_COMMITTER_NAME=Signing Test",
		"GIT_COMMITTER_EMAIL=signing@example.invalid",
		"DEVTOOLS_SIGNING_RUN_DIR="+a.Config.RunDir,
		"DEVTOOLS_SIGNING_PUBLIC_KEY="+a.Config.PublicKey,
		"DEVTOOLS_SIGNING_PROVIDER=file",
		"DEVTOOLS_SIGNING_KEY_FILE="+a.Config.KeyFile,
		"DEVTOOLS_SIGNING_CONFIG="+filepath.Join(repo, "absent.env"),
	)
	pub, err := os.ReadFile(a.Config.PublicKey)
	require.NoError(t, err)
	allowed := filepath.Join(repo, "allowed-signers")
	//nolint:gosec // The path is inside t.TempDir and the content is a generated public key.
	require.NoError(t, os.WriteFile(allowed, append([]byte("signing@example.invalid "), pub...), 0o600))
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Env = env
		cmd.Dir = repo
		data, cmdErr := cmd.CombinedOutput()
		require.NoError(t, cmdErr, string(data))
	}
	git("init", "-q")
	git(
		"-c",
		"core.hooksPath="+os.DevNull,
		"-c",
		"gpg.format=ssh",
		"-c",
		"gpg.ssh.program="+binary,
		"-c",
		"user.signingkey="+a.Config.PublicKey,
		"commit",
		"--allow-empty",
		"-S",
		"-m",
		"signed test",
	)
	git("-c", "gpg.format=ssh", "-c", "gpg.ssh.allowedSignersFile="+allowed, "verify-commit", "HEAD")
}
