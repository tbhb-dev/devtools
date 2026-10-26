// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbhb-dev/devtools/internal/agentrepo"
)

func fuzzFixture(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cmd := exec.CommandContext(t.Context(), "go")
	cmd.Args = append(cmd.Args, "build", "-o", filepath.Join(bin, name), "./testdata/fuzz/go.go")
	data, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", data)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return filepath.Join(t.TempDir(), "trace")
}

func fuzzTrace(t *testing.T, path string) [][]string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var result [][]string
	decoder := json.NewDecoder(bytes.NewReader(data))
	for decoder.More() {
		var args []string
		require.NoError(t, decoder.Decode(&args))
		result = append(result, args)
	}
	return result
}

func TestFuzzGoParity(t *testing.T) {
	trace := fuzzFixture(t)
	t.Setenv("FUZZ_TRACE", trace)
	for _, scenario := range []string{
		"success", "single", "empty-packages", "empty-targets", "packages-empty",
		"packages-partial", "targets-empty", "targets-partial", "target-failure",
	} {
		for _, explicit := range []bool{false, true} {
			t.Run(scenario+map[bool]string{false: "/default", true: "/explicit"}[explicit], func(t *testing.T) {
				t.Setenv("FUZZ_SCENARIO", scenario)
				fuzzParity(t, trace, scenario, explicit)
			})
		}
	}
}

func TestFuzzGoCapability(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var out, stderr bytes.Buffer
	require.Equal(t, 0, agentrepo.Run([]string{"fuzz-go", "--supported"}, nil, &out, &stderr))
	require.Empty(t, out.String())
	require.Empty(t, stderr.String())
}

func TestFuzzGoNativeSmoke(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n\ngo 1.24\n"), 0o600))
	source := "package example\nimport \"testing\"\nfunc FuzzEcho(f *testing.F) { f.Fuzz(func(t *testing.T, s string) { if string([]byte(s)) != s { t.Fatal(s) } }) }\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "echo_test.go"), []byte(source), 0o600))
	corpus := filepath.Join(root, "testdata", "fuzz", "FuzzEcho")
	require.NoError(t, os.MkdirAll(corpus, 0o700))
	seed := []byte("go test fuzz v1\nstring(\"seed\")\n")
	require.NoError(t, os.WriteFile(filepath.Join(corpus, "seed"), seed, 0o600))
	t.Chdir(root)
	t.Setenv("FUZZ_TIME", "1x")
	t.Setenv("GOFLAGS", "-parallel=1")
	var out, stderr bytes.Buffer
	require.Equal(t, 0, agentrepo.Run([]string{"fuzz-go"}, nil, &out, &stderr), "%s\n%s", &out, &stderr)
	require.Contains(t, out.String(), "==> example FuzzEcho\n")
	require.Contains(t, out.String(), "PASS")
	actual, err := os.ReadFile(filepath.Join(corpus, "seed"))
	require.NoError(t, err)
	require.Equal(t, seed, actual)
}

func fuzzParity(t *testing.T, trace, scenario string, explicit bool) {
	t.Helper()
	t.Setenv("FUZZ_TIME", "")
	args := []string{"fuzz-go"}
	path, duration := "./...", "30s"
	if explicit {
		path, duration = "./chosen path/...", "2s"
		args = append(args, path, "ignored")
		t.Setenv("FUZZ_TIME", duration)
	}
	require.NoError(t, os.WriteFile(trace, nil, 0o600))
	var out, stderr bytes.Buffer
	code := agentrepo.Run(args, nil, &out, &stderr)
	actualTrace := fuzzTrace(t, trace)
	require.NotEmpty(t, actualTrace, stderr.String())
	require.Equal(t, []string{"list", path}, actualTrace[0])
	expectedCode := 0
	if strings.Contains(scenario, "empty") {
		expectedCode = 1
	}
	if scenario == "target-failure" {
		expectedCode = 23
	}
	require.Equal(t, expectedCode, code)
	if code == 1 {
		require.Contains(t, stderr.String(), "no fuzz targets discovered under "+path+"\n")
	}
	fuzzAssertTrace(t, actualTrace, scenario, duration, out.String(), stderr.String())
	fuzzOracle(t, trace, args, code, out.String(), stderr.String(), actualTrace)
}

func fuzzAssertTrace(t *testing.T, actualTrace [][]string, scenario, duration, out, stderr string) {
	t.Helper()
	var targets []string
	for _, call := range actualTrace[1:] {
		if call[1] == "-list" {
			require.Equal(t, []string{"test", "-list", "^Fuzz", call[3]}, call)
			continue
		}
		require.Equal(t, []string{"test", "-run=^$", call[2], "-fuzztime=" + duration, call[4]}, call)
		targets = append(
			targets,
			call[4]+" "+strings.TrimSuffix(strings.TrimPrefix(call[2], "-fuzz=^"), "$"),
		)
	}
	expectedTargets := []string(nil)
	switch scenario {
	case "success":
		expectedTargets = []string{
			"example/one FuzzFirst",
			"example/one FuzzSecond",
			"example/two FuzzFirst",
		}
	case "packages-partial":
		expectedTargets = []string{"example/one FuzzFirst", "example/one FuzzSecond"}
	case "targets-partial":
		expectedTargets = []string{"example/one FuzzFirst", "example/two FuzzFirst"}
	case "target-failure", "single":
		expectedTargets = []string{"example/one FuzzFirst"}
	}
	require.Equal(t, expectedTargets, targets)
	for _, target := range expectedTargets {
		require.Contains(t, out, "==> "+target+"\n")
	}
	require.NotContains(t, stderr, "target discovery diagnostic")
}

func fuzzOracle(t *testing.T, trace string, args []string, code int, out, stderr string, actualTrace [][]string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	} // Frozen Bash oracle runs on Unix. Direct assertions run everywhere.
	require.NoError(t, os.WriteFile(trace, nil, 0o600))
	shell := exec.CommandContext(t.Context(), "bash")
	shell.Args = append(shell.Args, "testdata/fuzz/frozen.sh")
	shell.Args = append(shell.Args, args[1:]...)
	var oldOut, oldErr bytes.Buffer
	shell.Stdout, shell.Stderr = &oldOut, &oldErr
	err := shell.Run()
	oldCode := 0
	if err != nil {
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit)
		oldCode = exit.ExitCode()
	}
	require.Equal(t, oldCode, code)
	require.Equal(t, oldOut.String(), out)
	require.Equal(t, oldErr.String(), stderr)
	require.Equal(t, fuzzTrace(t, trace), actualTrace)
}
