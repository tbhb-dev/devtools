// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // Git blob identity of the pinned fixture, not a security check.
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type oracleResponse struct {
	Out    string `json:"out"`
	Err    string `json:"err"`
	Status int    `json:"status"`
}

type oracleOutcome struct {
	Out, Err      string
	Status, Reads int
}

func watchOracleResponse() int {
	dir := os.Getenv("WATCH_ORACLE_RESPONSES")
	data, err := os.ReadFile(filepath.Join(dir, "responses.json")) //nolint:gosec // Directory set by the parent test.
	if err != nil {
		return 9
	}
	var responses []oracleResponse
	if json.Unmarshal(data, &responses) != nil || len(responses) == 0 {
		return 9
	}
	n := 0
	countPath := filepath.Join(dir, "count")
	if count, readErr := os.ReadFile(countPath); readErr == nil { //nolint:gosec // Directory set by the parent test.
		parsed, convErr := strconv.Atoi(string(count))
		if convErr != nil {
			return 9
		}
		n = parsed
	}
	next := []byte(strconv.Itoa(n + 1))
	if os.WriteFile(countPath, next, 0o600) != nil { //nolint:gosec // Directory set by the parent test.
		return 9
	}
	response := responses[min(n, len(responses)-1)]
	fmt.Fprint(os.Stdout, response.Out)
	fmt.Fprint(os.Stderr, response.Err)
	return response.Status
}

// Only these two functions are called by the frozen watcher. Copy their exact
// baseline definitions from the shared library, without its unrelated startup
// probes or helper imports. The hash pins the supporting quota/access behavior
// as well as the watcher's blob, even on a shallow checkout.
func watchOracleFiles(t *testing.T, dest string) string {
	t.Helper()
	script, err := os.ReadFile("../../tests/fixtures/watch-oracle/watch-checks.sh")
	require.NoError(t, err)
	blob := fmt.Appendf(nil, "blob %d%c", len(script), 0)
	blob = append(blob, script...)
	blobID := sha1.Sum(blob) //nolint:gosec // Git blob identity of the pinned fixture, not a security check.
	require.Equal(t, "b2969dddf082d62698f2c4957240051ee31b8d6e", hex.EncodeToString(blobID[:]))
	library, err := os.ReadFile("../../packages/agents/source/lib/lib.sh")
	require.NoError(t, err)
	var support strings.Builder
	for _, name := range []string{"dt_gh", "dt_gh_login"} {
		_, tail, found := strings.Cut(string(library), name+"() {")
		require.True(t, found)
		body, _, found := strings.Cut(tail, "\n}\n")
		require.True(t, found)
		support.WriteString(name + "() {" + body + "\n}\n")
	}
	const wantSupport = "f259b49008191776f15c36b75edf364048ba7ef8a97c6a3e2361bc3326819bdb"
	supportHash := sha256.Sum256([]byte(support.String()))
	require.Equal(t, wantSupport, hex.EncodeToString(supportHash[:]))
	libPath := filepath.Join(dest, "lib.sh")
	library = []byte(support.String())
	require.NoError(t, os.WriteFile(libPath, library, 0o600))
	path := filepath.Join(dest, "watch-checks.sh")
	require.NoError(t, os.WriteFile(path, script, 0o600)) //nolint:gosec // Temporary test directory.
	return path
}

func TestWatchOracle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the frozen oracle requires Bash; parity covers Linux and macOS")
	}
	bin := t.TempDir()
	oracle := watchOracleFiles(t, t.TempDir())
	tools := map[string]string{}
	// Only the listed tools are on PATH, so no real network client can run.
	for _, name := range []string{"bash", "git", "jq", "dirname", "mktemp", "cat", "rm", "grep", "head", "sort"} {
		path, err := exec.LookPath(name)
		require.NoError(t, err)
		tools[name] = path
		require.NoError(t, os.Symlink(path, filepath.Join(bin, name)))
	}
	exe, err := os.Executable()
	require.NoError(t, err)
	for _, name := range []string{"gh", "sleep", "watch-command"} {
		require.NoError(t, os.Symlink(exe, filepath.Join(bin, name)))
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	git := func(args ...string) {
		t.Helper()
		//nolint:gosec // Allowlisted tool.
		cmd := exec.CommandContext(t.Context(), tools["git"], args...)
		cmd.Dir = root
		out, gitErr := cmd.CombinedOutput()
		require.NoError(t, gitErr, "%s", out)
	}
	git("init", "--quiet", "--initial-branch=topic")
	require.NoError(t, os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("base\n"), 0o600))
	git("add", "tracked.txt")
	git("-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "--quiet", "-m", "base")
	require.NoError(t, os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("edited\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "staged.txt"), []byte("staged\n"), 0o600))
	git("add", "staged.txt")
	require.NoError(t, os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("loose\n"), 0o600))
	// A local bare remote makes remote-ref preservation observable too.
	remote := t.TempDir()
	git("clone", "--bare", "--quiet", root, remote)
	git("remote", "add", "origin", remote)
	state := func() string {
		t.Helper()
		//nolint:gosec // Tool path from the fixed allowlist.
		refs := exec.CommandContext(t.Context(), tools["git"], "--git-dir", remote, "show-ref", "--head")
		out, refsErr := refs.Output()
		require.NoError(t, refsErr)
		index, indexErr := os.ReadFile(filepath.Join(root, ".git", "index"))
		require.NoError(t, indexErr)
		return watchGitState(t, tools["git"], root) + string(index) + string(out)
	}
	before := state()
	t.Setenv("PATH", bin)
	t.Setenv("WATCH_FIXTURE_LOG", t.TempDir())
	t.Setenv("PR_CHECKS_INTERVAL", "0")
	t.Setenv("PR_CHECKS_TIMEOUT", "0")
	t.Setenv("PR_CHECKS_APPEAR_TIMEOUT", "0")
	check := func(bucket string) string {
		return fmt.Sprintf(`{"name":"build","workflow":"ci","bucket":%q,"link":"/1"}`, bucket)
	}
	snapshot := func(checks ...string) oracleResponse {
		return oracleResponse{Out: "[" + strings.Join(checks, ",") + "]\n"}
	}
	pending, pass := snapshot(check("pending")), snapshot(check("pass"))
	run := func(shell bool, responses []oracleResponse) oracleOutcome {
		t.Helper()
		dir := t.TempDir()
		data, marshalErr := json.Marshal(responses)
		require.NoError(t, marshalErr)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "responses.json"), data, 0o600))
		t.Setenv("WATCH_ORACLE_RESPONSES", dir)
		//nolint:gosec // The test binary, symlinked.
		cmd := exec.CommandContext(t.Context(), filepath.Join(bin, "watch-command"), "checks", "watch", "7")
		if shell {
			//nolint:gosec // The pinned fixture copy.
			cmd = exec.CommandContext(t.Context(), tools["bash"], oracle, "7")
		}
		cmd.Dir = root
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		runErr := cmd.Run()
		status := 0
		if runErr != nil {
			var exit *exec.ExitError
			require.ErrorAs(t, runErr, &exit)
			status = exit.ExitCode()
		}
		count, readErr := os.ReadFile(filepath.Join(dir, "count"))
		require.NoError(t, readErr)
		reads, convErr := strconv.Atoi(string(count))
		require.NoError(t, convErr)
		require.Equal(t, before, state(), "AC2: content, index, local and remote refs")
		return oracleOutcome{out.String(), stderr.String(), status, reads}
	}
	for _, c := range []struct {
		name          string
		responses     []oracleResponse
		status, reads int
		appear        string
	}{
		{"passing", []oracleResponse{pending, pending, snapshot(check("pass"), strings.ReplaceAll(check("skipping"), "build", "docs"))}, 0, 3, "0"},
		{"failing", []oracleResponse{pending, snapshot(check("fail"))}, 1, 2, "0"},
		{"cancelled", []oracleResponse{pending, snapshot(check("cancel"), strings.ReplaceAll(check("pending"), "build", "slow")), snapshot(check("cancel"))}, 1, 3, "0"},
		{"rerun-recovered", []oracleResponse{pending, snapshot(check("fail"), strings.ReplaceAll(check("pending"), "build", "slow")), snapshot(check("fail"), strings.ReplaceAll(check("pass"), "/1", "/9"))}, 0, 3, "0"},
		{"empty-then-appearing", []oracleResponse{snapshot(), pending, pass}, 0, 3, "5"},
		{"unfinished-until-timeout", []oracleResponse{pending}, 2, 2, "0"},
		{"read-error", []oracleResponse{pending, {Err: "HTTP 502: Bad Gateway\n", Status: 1}}, 2, 2, "0"},
		{"quota-primary", []oracleResponse{pending, {Err: "gh: API rate limit exceeded for user ID 1. (HTTP 403)\n", Status: 1}}, 76, 2, "0"},
		{"quota-secondary", []oracleResponse{pending, {Err: "gh: secondary rate limit\n", Status: 1}}, 76, 2, "0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("PR_CHECKS_APPEAR_TIMEOUT", c.appear)
			// These sequences need one settling step, with zero-duration sleeps.
			if c.name == "passing" || c.name == "cancelled" || c.name == "rerun-recovered" {
				t.Setenv("PR_CHECKS_TIMEOUT", "1")
			}
			shell := run(true, c.responses)
			command := run(false, c.responses)
			require.Equal(t, shell, command, "AC1: stdout, stderr, status and check reads")
			require.Equal(t, c.status, command.Status)
			require.Equal(t, c.reads, command.Reads)
		})
	}
	t.Run("intentional-empty-after-registration-divergence", func(t *testing.T) {
		t.Setenv("PR_CHECKS_INTERVAL", "1")
		t.Setenv("PR_CHECKS_TIMEOUT", "1")
		recovering := []oracleResponse{pending, snapshot(), pass}
		responses := recovering[:2]
		shell, command := run(true, responses), run(false, responses)
		require.Equal(t, oracleOutcome{"WATCHING #7\nALL GREEN #7\n", "", 0, 2}, shell)
		timedOut := "WATCHING #7\nTIMEOUT #7 after 1s with checks still running\n"
		require.Equal(t, oracleOutcome{timedOut, "", 2, 3}, command)
		// Go can recover on its extra read. The shell has already exited green.
		shell, command = run(true, recovering), run(false, recovering)
		require.Equal(t, 2, shell.Reads)
		require.Equal(t, oracleOutcome{"WATCHING #7\nPASS  build\nALL GREEN #7\n", "", 0, 3}, command)
	})
}
