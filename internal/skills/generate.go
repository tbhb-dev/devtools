// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package skills renders canonical skill sources into harness packages.
package skills

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	source                     = "packages/agents/source"
	tokens                     = "tokens.json"
	regularMode    os.FileMode = 0o644
	executableMode os.FileMode = 0o755
)

var (
	errMissingInclude = errors.New("which does not exist")
	errCollision      = errors.New("exists in both")
)

type file struct {
	data []byte
	mode os.FileMode
}

type generator struct {
	root        string
	out         io.Writer
	readDir     func(string) ([]os.DirEntry, error)
	readFile    func(string) ([]byte, error)
	mkdir       func(string, os.FileMode) error
	writeFile   func(string, []byte, os.FileMode) error
	fingerprint func(string) (map[string]file, error)
	compare     func(map[string]file, map[string]file) ([]string, error)
	modes       map[string]os.FileMode
	findings    int
}

func newGenerator(root string, out io.Writer) *generator {
	g := &generator{
		root:      root,
		out:       out,
		readDir:   os.ReadDir,
		readFile:  os.ReadFile,
		mkdir:     os.MkdirAll,
		writeFile: os.WriteFile,
		compare:   compare,
	}
	g.fingerprint = func(dir string) (map[string]file, error) { return g.scan(dir, true) }
	return g
}

// Run generates packages, or reports drift without writing repository files.
func Run(root string, check bool, out, stderr io.Writer) int {
	g := newGenerator(root, out)
	// Windows doesn't expose executable bits. Git's index preserves the
	// canonical modes for tracked inputs and generated output on that platform.
	if runtime.GOOS == "windows" {
		data, err := exec.CommandContext(context.Background(), "git", "-C", root, "ls-files", "--stage", "-z").Output()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		g.modes = make(map[string]os.FileMode)
		for record := range strings.SplitSeq(string(data), "\x00") {
			meta, path, ok := strings.Cut(record, "\t")
			if !ok {
				continue
			}
			mode := regularMode
			if strings.HasPrefix(meta, "100755 ") {
				mode = executableMode
			}
			g.modes[path] = mode
		}
	}
	if err := g.generate(check); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if g.findings > 0 {
		fmt.Fprintf(out, "TOTAL: %d finding(s)\n", g.findings)
		return 1
	}
	return 0
}

func (g *generator) path(path string) string { return filepath.Join(g.root, filepath.FromSlash(path)) }

func (g *generator) tree(dir string) (map[string]file, error) { return g.scan(dir, false) }

func (g *generator) scan(dir string, ignoreTokens bool) (map[string]file, error) {
	files := make(map[string]file)
	if err := g.walk(dir, "", files, ignoreTokens); err != nil {
		return nil, err
	}
	return files, nil
}

func (g *generator) walk(dir, rel string, files map[string]file, ignoreTokens bool) error {
	entries, err := g.readDir(g.path(filepath.Join(dir, rel)))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := filepath.Join(rel, entry.Name())
		if entry.IsDir() {
			if walkErr := g.walk(dir, name, files, ignoreTokens); walkErr != nil {
				return walkErr
			}
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return fmt.Errorf("inspect %s: %w", name, infoErr)
		}
		if !info.Mode().IsRegular() || (ignoreTokens && rel == "" && entry.Name() == tokens) {
			continue
		}
		path := filepath.ToSlash(filepath.Join(dir, name))
		f, readErr := g.read(path)
		if readErr != nil {
			return readErr
		}
		files[filepath.ToSlash(name)] = f
	}
	return nil
}

func (g *generator) read(path string) (file, error) {
	data, err := g.readFile(g.path(path))
	if err != nil {
		return file{}, err
	}
	info, err := os.Stat(g.path(path))
	if err != nil {
		return file{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	mode := info.Mode().Perm()
	if indexed, ok := g.modes[path]; ok {
		mode = indexed
	}
	return file{data, mode}, nil
}

func (g *generator) fragment(template, src, part string) ([]byte, error) {
	info, err := os.Stat(g.path(src + "/" + part))
	if os.IsNotExist(err) || (err == nil && !info.Mode().IsRegular()) {
		return nil, fmt.Errorf("generate-skills: %s includes %s, %w", template, part, errMissingInclude)
	}
	if err != nil {
		return nil, fmt.Errorf("inspect include: %w", err)
	}
	return g.readFile(g.path(src + "/" + part))
}

func (g *generator) render(template, src string) ([]byte, error) {
	data, err := g.readFile(g.path(template))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	lines := bytes.Split(data, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		if part, ok := strings.CutPrefix(string(line), "@include "); ok {
			fragment, fragmentErr := g.fragment(template, src, part)
			if fragmentErr != nil {
				return nil, fragmentErr
			}
			out.Write(fragment)
		} else {
			out.Write(line)
			out.WriteByte('\n')
		}
	}
	return out.Bytes(), nil
}

func (g *generator) build(src, harness string) (map[string]file, error) {
	manifest, err := g.render(src+"/SKILL."+harness+".md", src)
	if err != nil {
		return nil, err
	}
	result := map[string]file{"SKILL.md": {manifest, 0o644}}
	for _, tree := range []string{"shared", harness} {
		if mergeErr := g.merge(result, src, harness, tree); mergeErr != nil {
			return nil, mergeErr
		}
	}
	helpers := []string{}
	if mentions(result, "lib.sh") {
		helpers = append(helpers, "lib.sh", "gates.py", "agent.sh", "signing.sh")
	}
	// The shell searches again after adding the library helpers.
	for _, name := range helpers {
		if helperErr := g.helper(result, name); helperErr != nil {
			return nil, helperErr
		}
	}
	if mentions(result, "worktree-ready.sh") {
		if helperErr := g.helper(result, "worktree-ready.sh"); helperErr != nil {
			return nil, helperErr
		}
	}
	return result, nil
}

func (g *generator) merge(result map[string]file, src, harness, tree string) error {
	dir := src + "/" + tree
	// The frozen find doesn't follow a symlink at the content root.
	info, err := os.Lstat(g.path(dir))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect source: %w", err)
	}
	if !info.IsDir() {
		return nil
	}
	files, err := g.tree(dir)
	if err != nil {
		return err
	}
	for _, name := range names(files) {
		if outputExists(result, name) {
			return fmt.Errorf("generate-skills: %s %w shared/ and %s/ of %s", name, errCollision, harness, src)
		}
		result[name] = files[name]
	}
	return nil
}

func outputExists(files map[string]file, path string) bool {
	if _, ok := files[path]; ok {
		return true
	}
	for name := range files {
		if strings.HasPrefix(name, path+"/") {
			return true
		}
	}
	return false
}

// The frozen listing extracts the first space-delimited field for findings.
func findingName(path string) string { name, _, _ := strings.Cut(path, " "); return name }

func (g *generator) helper(files map[string]file, name string) error {
	f, err := g.read(source + "/lib/" + name)
	if err != nil {
		return err
	}
	files["scripts/"+name] = f
	return nil
}

func mentions(files map[string]file, text string) bool {
	for name, f := range files {
		if strings.HasPrefix(name, "scripts/") && bytes.Contains(f.data, []byte(text)) {
			return true
		}
	}
	return false
}

func names(files map[string]file) []string {
	result := make([]string, 0, len(files))
	for name := range files {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func compare(want, got map[string]file) ([]string, error) {
	changed := make(map[string]file)
	for name, f := range want {
		if name == tokens {
			continue
		}
		other, ok := got[name]
		if !ok || (f.mode&0o111 != 0) != (other.mode&0o111 != 0) || !bytes.Equal(f.data, other.data) {
			changed[findingName(name)] = f
		}
	}
	for name, f := range got {
		if name == tokens {
			continue
		}
		if _, ok := want[name]; !ok {
			changed[findingName(name)] = f
		}
	}
	return names(changed), nil
}

func (g *generator) report(path, rule, message string) {
	fmt.Fprintf(g.out, "%s:1 [error] %s  %s\n", path, rule, message)
	g.findings++
}

func directory(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect directory: %w", err)
	}
	return info.IsDir(), nil
}

func (g *generator) generate(check bool) error {
	entries, err := g.readDir(g.path(source + "/skills"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		isDir, dirErr := directory(g.path(source + "/skills/" + entry.Name()))
		if dirErr != nil {
			return dirErr
		}
		if !isDir {
			continue
		}
		for _, harness := range []string{"claude", "codex"} {
			if skillErr := g.skill(entry.Name(), harness, check); skillErr != nil {
				return skillErr
			}
		}
	}
	return nil
}

func (g *generator) skill(name, harness string, check bool) error {
	src := source + "/skills/" + name
	info, err := os.Stat(g.path(src + "/SKILL." + harness + ".md"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect template: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	want, err := g.build(src, harness)
	if err != nil {
		return err
	}
	out := "packages/agents/" + harness + "/.apm/skills/" + name
	if check {
		return g.check(src, out, want)
	}
	if err = g.publish(out, want); err != nil {
		return err
	}
	fmt.Fprintf(g.out, "generated %s\n", out)
	return nil
}

func (g *generator) check(src, out string, want map[string]file) error {
	exists, err := directory(g.path(out))
	if err != nil {
		return err
	}
	if !exists {
		g.report(out, "not-generated", "missing; run mise run generate-skills")
		return nil
	}
	got, err := g.fingerprint(out)
	if err != nil {
		return err
	}
	g.outputModes(out, want, got)
	changed, err := g.compare(want, got)
	if err != nil {
		return err
	}
	for _, name := range changed {
		g.report(
			out+"/"+name,
			"generated-drift",
			"differs from what "+src+" generates; edit the source, then run mise run generate-skills",
		)
	}
	return nil
}

// For untracked Windows output there is no executable bit to inspect. Use
// the generated mode until an index entry can supply the recorded decision.
func (g *generator) outputModes(out string, want, got map[string]file) {
	if g.modes == nil {
		return
	}
	for name, f := range got {
		if _, indexed := g.modes[out+"/"+name]; indexed {
			continue
		}
		if expected, ok := want[name]; ok {
			f.mode = expected.mode
			got[name] = f
		}
	}
}

func (g *generator) publish(out string, files map[string]file) error {
	if err := g.mkdir(g.path(out), executableMode); err != nil {
		return err
	}
	entries, err := g.readDir(g.path(out))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != tokens {
			if removeErr := os.RemoveAll(g.path(out + "/" + entry.Name())); removeErr != nil {
				return fmt.Errorf("remove stale output: %w", removeErr)
			}
		}
	}
	for _, name := range names(files) {
		f := files[name]
		path := g.path(out + "/" + name)
		if mkdirErr := g.mkdir(filepath.Dir(path), executableMode); mkdirErr != nil {
			return mkdirErr
		}
		if writeErr := g.writeFile(path, f.data, f.mode); writeErr != nil {
			return writeErr
		}
		if chmodErr := os.Chmod(path, f.mode); chmodErr != nil {
			return fmt.Errorf("set output mode: %w", chmodErr)
		}
	}
	return nil
}
