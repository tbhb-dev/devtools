// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package agentrepo

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func proseCommand(code *int) *cobra.Command {
	group := &cobra.Command{Use: "prose", Short: "Manage prose workflow evidence"}
	group.AddCommand(&cobra.Command{
		Use:                "check-suppressions --baseline TARGET | --diff STATE [TARGET] | --cleanup STATE",
		Short:              "Record, read, or remove a prose baseline",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			p := proseState{out: cmd.OutOrStdout(), stderr: cmd.ErrOrStderr()}
			p.run = diagnosisRunner(cmd.Context())
			*code = p.execute(args)
		},
	})
	return group
}

type proseState struct {
	root, gitDir string
	out, stderr  io.Writer
	run          func(dir, tool string, args ...string) ([]byte, error)
}

func (p *proseState) refuse(message string) int {
	fmt.Fprintln(p.stderr, "check-suppressions: "+message)
	const invalidState = 2
	return invalidState
}

func (p *proseState) git(args ...string) ([]byte, error) {
	return p.run(p.root, "git", append([]string{
		"--no-pager", "-c", "core.quotePath=false", "-c", "color.ui=false",
		"-c", "color.diff=false", "-c", "color.status=false",
		"-c", "diff.noprefix=false", "-c", "diff.mnemonicPrefix=false",
	}, args...)...)
}

func (p *proseState) execute(args []string) int {
	if problem := p.checkout(); problem != "" {
		return p.refuse(problem)
	}
	if len(args) == 0 {
		return p.refuse("expected --baseline, --diff, or --cleanup")
	}
	arg := ""
	if len(args) > 1 {
		arg = args[1]
	}
	if args[0] == "--baseline" {
		return p.baseline(arg)
	}
	if args[0] != "--diff" && args[0] != "--cleanup" {
		return p.refuse("expected --baseline, --diff, or --cleanup")
	}
	state, problem := p.load(arg)
	if problem != "" {
		return p.refuse(problem)
	}
	if args[0] == "--cleanup" {
		if err := os.RemoveAll(state); err != nil {
			return p.refuse(err.Error())
		}
		return 0
	}
	supplied := ""
	const targetIndex = 2
	if len(args) > targetIndex {
		supplied = args[targetIndex]
	}
	return p.targetDiff(state, supplied)
}

func (p *proseState) checkout() string {
	root, err := p.git("rev-parse", "--show-toplevel")
	if err != nil {
		return err.Error()
	}
	p.root, err = filepath.EvalSymlinks(strings.TrimSuffix(string(root), "\n"))
	if err != nil {
		return err.Error()
	}
	gitDir, err := p.git("rev-parse", "--absolute-git-dir")
	if err != nil {
		return err.Error()
	}
	p.gitDir = strings.TrimSuffix(string(gitDir), "\n")
	return ""
}

func (p *proseState) targetDiff(state, supplied string) int {
	targetBytes, err := os.ReadFile(filepath.Join(state, "target"))
	if err != nil {
		return p.refuse(err.Error())
	}
	target, _, _ := strings.Cut(string(targetBytes), "\n")
	if supplied != "" {
		supplied = p.canonicalTarget(supplied)
		if supplied != target {
			return p.refuse(fmt.Sprintf("state belongs to %s, not %s", target, supplied))
		}
	}
	return p.diff(state, target)
}

func (p *proseState) canonicalTarget(target string) string {
	// The shell changes to the checkout root first, so relative targets resolve there.
	target = p.path(target)
	if parent, err := filepath.EvalSymlinks(filepath.Dir(target)); err == nil {
		target = filepath.Join(parent, filepath.Base(target))
	}
	prefix := p.root + string(filepath.Separator)
	target, _ = strings.CutPrefix(target, prefix)
	return filepath.ToSlash(target)
}

func (p *proseState) load(state string) (string, string) {
	state = filepath.FromSlash(state)
	if strings.HasPrefix(state, filepath.Join(".fix-prose", "run.")) {
		state = p.root + string(filepath.Separator) + state
	}
	if !strings.HasPrefix(state, filepath.Join(p.root, ".fix-prose")+string(filepath.Separator)+"run.") &&
		!strings.HasPrefix(state, filepath.Join(p.gitDir, "fix-prose")+string(filepath.Separator)+"run.") {
		if state == "" {
			state = "(none)"
		}
		return "", "invalid workflow state: " + state
	}
	info, err := os.Stat(filepath.Join(state, "target"))
	if err != nil || info.Size() == 0 {
		return "", "workflow state is missing its target: " + state
	}
	return state, ""
}

func (p *proseState) baseline(target string) int {
	if target == "" {
		return p.refuse("--baseline needs a target path")
	}
	target = p.canonicalTarget(target)
	base := filepath.Join(p.root, ".fix-prose")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return p.refuse(err.Error())
	}
	ignore := filepath.Join(base, ".gitignore")
	if _, statErr := os.Stat(ignore); os.IsNotExist(statErr) {
		if err := os.WriteFile(ignore, []byte("*\n"), 0o600); err != nil {
			return p.refuse(err.Error())
		}
	}
	state := filepath.Join(base, fmt.Sprintf("run.%x", sha256.Sum256([]byte(target))))
	if err := os.Mkdir(state, 0o700); err != nil {
		const guidance = "an active workflow already owns %s; if that run is abandoned, clear it with: check-suppressions.sh --cleanup %s"
		return p.refuse(fmt.Sprintf(guidance, target, state))
	}
	if err := p.snapshot(state, target); err != nil {
		return p.refuse(err.Error())
	}
	fmt.Fprintln(p.out, "state: "+state)
	return 0
}

func proseWrite(state, name string, data []byte) error {
	root, err := os.OpenRoot(state)
	if err != nil {
		return fmt.Errorf("open workflow: %w", err)
	}
	defer root.Close()
	const stateFileMode = 0o600
	if writeErr := root.WriteFile(name, data, stateFileMode); writeErr != nil {
		return fmt.Errorf("record %s: %w", name, writeErr)
	}
	return nil
}

func (p *proseState) snapshot(state, target string) error {
	if err := proseWrite(state, "target", []byte(target+"\n")); err != nil {
		return err
	}
	digest, err := p.digest(target)
	if err != nil {
		return err
	}
	if writeErr := proseWrite(state, "digest", digest); writeErr != nil {
		return writeErr
	}
	if docErr := p.document(state, target); docErr != nil {
		return docErr
	}
	return p.dictionaries(state)
}

func (p *proseState) document(state, target string) error {
	data, err := os.ReadFile(p.path(target))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read target: %w", err)
	}
	directives := regexp.MustCompile(
		`vale[\t\v\f\r ]+(off|on)|vale[\t\v\f\r ]+[A-Za-z][A-Za-z0-9._-]*[\t\v\f\r ]*=[\t\v\f\r ]*([Nn][Oo]|off)|cspell:|cSpell:|spell-checker:|markdownlint-disable|rumdl-disable`,
	)
	var found strings.Builder
	for i, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if directives.MatchString(line) {
			fmt.Fprintf(&found, "%d:%s\n", i+1, line)
		}
	}
	if writeErr := proseWrite(state, "directives", []byte(found.String())); writeErr != nil {
		return writeErr
	}
	if data != nil {
		return proseWrite(state, "original", data)
	}
	return nil
}

func (p *proseState) dictionaries(state string) error {
	dictionaries, err := filepath.Glob(filepath.Join(p.root, ".vale/config/vocabularies/*/accept.txt"))
	if err != nil {
		return fmt.Errorf("find dictionaries: %w", err)
	}
	dictionaries = append(dictionaries, filepath.Join(p.root, ".cspell-words.txt"))
	for _, file := range dictionaries {
		data, readErr := os.ReadFile(file)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return fmt.Errorf("read dictionary: %w", readErr)
		}
		rel, relErr := filepath.Rel(p.root, file)
		if relErr != nil {
			return fmt.Errorf("locate dictionary: %w", relErr)
		}
		dest := filepath.Join("watched", rel)
		if mkdirErr := os.MkdirAll(filepath.Dir(filepath.Join(state, dest)), 0o700); mkdirErr != nil {
			return fmt.Errorf("create dictionary directory: %w", mkdirErr)
		}
		if writeErr := proseWrite(state, dest, data); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func (p *proseState) path(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(p.root, path)
}

func proseStyles(root string) (map[string]bool, error) {
	paths := map[string]bool{}
	err := fs.WalkDir(os.DirFS(root), ".vale", func(path string, d fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			paths[path] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk styles: %w", err)
	}
	return paths, nil
}

func (p *proseState) watchedPaths() (map[string]bool, error) {
	paths, err := proseStyles(p.root)
	if err != nil {
		return nil, err
	}
	for _, file := range []string{".vale.ini", ".cspell.jsonc", ".cspell-words.txt", ".pre-commit-config.yaml", "mise.toml", "Justfile"} {
		if info, statErr := os.Stat(p.path(file)); statErr == nil && info.Mode().IsRegular() {
			paths[file] = true
		}
	}
	for _, args := range [][]string{{"diff", "--no-ext-diff", "HEAD", "--name-only"}, {"ls-files", "--others", "--exclude-standard"}} {
		data, gitErr := p.git(args...)
		if gitErr != nil {
			return nil, gitErr
		}
		for path := range strings.SplitSeq(strings.TrimSuffix(string(data), "\n"), "\n") {
			if path != "" {
				paths[path] = true
			}
		}
	}
	return paths, nil
}

func (p *proseState) digest(target string) ([]byte, error) {
	paths, err := p.watchedPaths()
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(paths))
	for path := range paths {
		if path != target && !strings.HasPrefix(path, ".fix-prose/") {
			keys = append(keys, path)
		}
	}
	sort.Strings(keys)
	var out bytes.Buffer
	for _, path := range keys {
		data, readErr := os.ReadFile(p.path(path))
		hash := fmt.Sprintf("%x", [sha256.Size]byte{})
		if readErr == nil {
			hash = fmt.Sprintf("%x", sha256.Sum256(data))
		} else if !os.IsNotExist(readErr) {
			return nil, fmt.Errorf("read watched path: %w", readErr)
		}
		fmt.Fprintf(&out, "%s  %s\n", hash, path)
	}
	return out.Bytes(), nil
}

func (p *proseState) diff(state, target string) int {
	original := filepath.Join(state, "original")
	if info, err := os.Stat(original); err != nil || !info.Mode().IsRegular() {
		return p.refuse("baseline kept no copy of " + target)
	}
	tool := "diff"
	if runtime.GOOS == "windows" {
		data, err := p.git("--exec-path")
		if err != nil {
			return p.refuse(err.Error())
		}
		tool = filepath.Join(strings.TrimSpace(string(data)), "../../../usr/bin/diff.exe")
	}
	data, err := p.run(p.root, tool, "-u", original, target)
	fmt.Fprint(p.out, string(data))
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		fmt.Fprint(p.stderr, string(exit.Stderr))
	} else if err != nil {
		return p.refuse(err.Error())
	}
	return 0
}
