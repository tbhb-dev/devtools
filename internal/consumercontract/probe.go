// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	commitScripts = "commit/scripts"
	mergeScripts  = "merge-pr/scripts"
	// altValue is an arbitrary nonempty value. A reader that takes it accepts
	// the nonempty class rather than one exact string.
	altValue = "probe-alternate"
	// fillerValue satisfies a pattern setting without selecting any path.
	fillerValue = "^$"
	// constitutionalKey is read by the merge reader from the base blob of
	// the conf rather than from the values the shell reader hands it.
	constitutionalKey = "constitutional_paths"
	confDir           = ".agents"
	confName          = "agent-skills.conf"
)

// skillDirs lists the directories where a tree keeps its reader scripts:
// the Claude package first, then the Codex one.
func skillDirs() []string { return []string{".claude/skills", ".agents/skills"} }

// ErrNoReader reports a release tree without the shell reader to probe.
var ErrNoReader = errors.New("release tree has no repository configuration reader")

var mergeKeyLoop = regexp.MustCompile(`for key in ((?:[a-z_]+ )+[a-z_]+); do\n\s+value=\$\(dt_repo_conf "\$key"\)`)

// Prober measures what a release's own readers accept. Root is a tree
// holding that release's skill scripts, such as an extracted tag.
type Prober struct {
	Root string

	skills    string
	home      string
	shellRepo string
	mergeKeys []string
	hasMerge  bool
	mergeMemo map[string]bool
	scaffold  []Key
}

// newProber builds the probe for the tree at root. The returned function
// removes its scratch directory.
func newProber(ctx context.Context, root string) (*Prober, func(), error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve %s: %w", root, err)
	}
	scratch, err := os.MkdirTemp("", "consumercontract-probe-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create probe directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(scratch) }
	prober := &Prober{Root: root, home: scratch, mergeMemo: map[string]bool{}}
	if err = prober.setup(ctx); err != nil {
		cleanup()
		return nil, nil, err
	}
	return prober, cleanup, nil
}

// ProbeReader returns the states each key accepts, measured by running the
// release's shell reader and merge policy reader against scratch trees.
func ProbeReader(ctx context.Context, root string, keys []Key) (Matrix, error) {
	prober, cleanup, err := newProber(ctx, root)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if err = prober.discoverContext(ctx, keys); err != nil {
		return nil, err
	}
	matrix := Matrix{}
	for _, key := range keys {
		var states StateSet
		states, err = prober.probeKey(ctx, key)
		if err != nil {
			return nil, err
		}
		matrix[key.ID()] = states
	}
	return matrix, nil
}

func (p *Prober) setup(ctx context.Context) error {
	for _, dir := range skillDirs() {
		if _, err := os.Stat(filepath.Join(p.Root, dir, commitScripts, "lib.sh")); err == nil {
			p.skills = filepath.Join(p.Root, dir)
			break
		}
	}
	if p.skills == "" {
		return fmt.Errorf("%w: %s", ErrNoReader, p.Root)
	}
	p.shellRepo = filepath.Join(p.home, "shell")
	if err := os.MkdirAll(filepath.Join(p.shellRepo, confDir), 0o750); err != nil {
		return fmt.Errorf("create probe tree: %w", err)
	}
	if _, err := git(ctx, p.shellRepo, hermeticEnv(p.home), "init", "-q"); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(p.skills, mergeScripts, "merge-agent.sh"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read merge reader: %w", err)
	}
	found := mergeKeyLoop.FindSubmatch(data)
	if found == nil {
		return fmt.Errorf("%w: merge reader names no setting loop", ErrNoReader)
	}
	p.mergeKeys = strings.Fields(string(found[1]))
	p.hasMerge = true
	return nil
}

// probeKey measures the four states one key can be in.
func (p *Prober) probeKey(ctx context.Context, key Key) (StateSet, error) {
	states := StateSet{}
	for _, probe := range []struct{ state, line string }{
		{StateAbsent, ""},
		{StateEmpty, key.Name + " ="},
		{StateNonempty, key.Name + " = " + altValue},
	} {
		accepted, err := p.accepts(ctx, key.Name, probe.line)
		if err != nil {
			return nil, err
		}
		if accepted {
			states.Add(probe.state)
		}
	}
	if !states[StateNonempty] && key.Sample != "" {
		accepted, err := p.accepts(ctx, key.Name, key.Name+" = "+key.Sample)
		if err != nil {
			return nil, err
		}
		if accepted {
			states.Add(ValueState(key.Sample))
		}
	}
	return states, nil
}

// accepts says whether every reader takes a conf holding the one probed
// line, plus a valid filler for every setting the merge reader requires and
// the scaffold the reader needs beside any key.
func (p *Prober) accepts(ctx context.Context, name, line string) (bool, error) {
	var conf strings.Builder
	conf.WriteString(p.filler(name))
	for _, key := range p.scaffold {
		if key.Name != name {
			conf.WriteString(key.Name + " = " + key.Sample + "\n")
		}
	}
	if line != "" {
		conf.WriteString(line + "\n")
	}
	return p.acceptsConf(ctx, conf.String(), []string{name})
}

// filler writes a valid value for each setting the merge reader requires.
func (p *Prober) filler(except string) string {
	var conf strings.Builder
	for _, key := range p.mergeKeys {
		if key != except {
			conf.WriteString(key + " = " + fillerValue + "\n")
		}
	}
	return conf.String()
}

// acceptsConf runs both readers over a whole conf, asking the shell reader
// for the settings in names and the merge settings.
func (p *Prober) acceptsConf(ctx context.Context, conf string, names []string) (bool, error) {
	values, ok, err := p.shell(ctx, append(slices.Clone(names), p.mergeKeys...), conf)
	if err != nil || !ok || !p.hasMerge {
		return ok && err == nil, err
	}
	return p.merge(ctx, conf, values)
}

// shell runs the release's `dt_repo_conf` over the settings in names and
// returns their values. Any rejection of the conf fails the whole run.
func (p *Prober) shell(ctx context.Context, names []string, conf string) (map[string]string, bool, error) {
	path := filepath.Join(p.shellRepo, confDir, confName)
	if err := os.WriteFile(path, []byte(conf), 0o600); err != nil {
		return nil, false, fmt.Errorf("write probe conf: %w", err)
	}
	script := `source "$1" || exit 70; shift; for k in "$@"; do v=$(dt_repo_conf "$k") || exit 1; printf '%s\n' "$v"; done`
	args := append([]string{"-c", script, "_", filepath.Join(p.skills, commitScripts, "lib.sh")}, names...)
	env := hermeticEnv(p.home, "_DEVTOOLS_AGENTREPO_CONFIG=false")
	res, err := run(ctx, p.shellRepo, env, "", "bash", args...)
	if err != nil {
		return nil, false, err
	}
	if !res.ok {
		// Exit 1 is the reader refusing the conf. Any other status means the
		// reader never ran, and reading that as a refusal would hide it.
		if res.code != 1 {
			return nil, false, fmt.Errorf("%w: the reader at %s exited %d", ErrCommand, p.skills, res.code)
		}
		return nil, false, nil
	}
	lines := strings.Split(strings.TrimSuffix(res.stdout, "\n"), "\n")
	values := map[string]string{}
	for index, key := range names {
		if index < len(lines) {
			values[key] = lines[index]
		}
	}
	return values, true, nil
}

// merge runs the release's merge policy inspection against a scratch
// change, the way merge-agent.sh does: settings from the shell reader, and
// the constitutional pattern from the conf at the base commit.
func (p *Prober) merge(ctx context.Context, conf string, values map[string]string) (bool, error) {
	dict := map[string]string{}
	for _, key := range p.mergeKeys {
		dict[key] = values[key]
	}
	payload, err := json.Marshal(dict)
	if err != nil {
		return false, fmt.Errorf("encode merge settings: %w", err)
	}
	memo := string(payload) + "|" + confLine(conf, constitutionalKey)
	if accepted, seen := p.mergeMemo[memo]; seen {
		return accepted, nil
	}
	repo, err := p.mergeRepo(ctx, conf)
	if err != nil {
		return false, err
	}
	policy := filepath.Join(p.skills, mergeScripts, "merge-policy.py")
	res, err := run(
		ctx, repo, hermeticEnv(p.home), string(payload),
		"uv", "run", "--quiet", "--script", policy, "inspect", "main", "change",
	)
	if err != nil {
		return false, err
	}
	p.mergeMemo[memo] = res.ok
	return res.ok, nil
}

// mergeRepo builds the scratch repository the merge policy inspects: a base
// commit holding conf and a change commit on top of it.
func (p *Prober) mergeRepo(ctx context.Context, conf string) (string, error) {
	repo, err := os.MkdirTemp(p.home, "merge-*")
	if err != nil {
		return "", fmt.Errorf("create merge probe: %w", err)
	}
	env := hermeticEnv(p.home)
	steps := func(commands ...[]string) error {
		for _, args := range commands {
			if _, stepErr := git(ctx, repo, env, args...); stepErr != nil {
				return stepErr
			}
		}
		return nil
	}
	if err = steps([]string{"init", "-q", "-b", "main"}); err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Join(repo, confDir), 0o750); err != nil {
		return "", fmt.Errorf("create merge probe tree: %w", err)
	}
	if err = writeProbeFile(filepath.Join(repo, confDir, confName), conf); err != nil {
		return "", err
	}
	base := [][]string{{"add", "-A"}, {"commit", "-q", "-m", "base"}, {"checkout", "-q", "-b", "change"}}
	if err = steps(base...); err != nil {
		return "", err
	}
	if err = writeProbeFile(filepath.Join(repo, "change.txt"), "change\n"); err != nil {
		return "", err
	}
	if err = steps([]string{"add", "-A"}, []string{"commit", "-q", "-m", "change"}); err != nil {
		return "", err
	}
	return repo, nil
}

func writeProbeFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write probe file %s: %w", path, err)
	}
	return nil
}

// confLine returns the raw line that sets a key, or "" when none does.
func confLine(conf, name string) string {
	for line := range strings.SplitSeq(conf, "\n") {
		if key, _, found := strings.Cut(line, "="); found && strings.TrimSpace(key) == name {
			return line
		}
	}
	return ""
}

// AcceptsConf says whether a release's readers accept a whole conf as it
// stands: the shell reader over every setting in it, and the merge policy
// reader over the settings that reader requires.
func AcceptsConf(ctx context.Context, root, conf string) (bool, error) {
	prober, cleanup, err := newProber(ctx, root)
	if err != nil {
		return false, err
	}
	defer cleanup()
	names := slices.Clone(prober.mergeKeys)
	for line := range strings.SplitSeq(conf, "\n") {
		if key, _, found := strings.Cut(line, "="); found && !strings.HasPrefix(strings.TrimSpace(key), "#") {
			names = append(names, strings.TrimSpace(key))
		}
	}
	values, ok, err := prober.shell(ctx, names, conf)
	if err != nil || !ok || !prober.hasMerge {
		return ok && err == nil, err
	}
	return prober.merge(ctx, conf, values)
}

// discoverContext finds the settings that must appear with a probed one for
// the readers to take the conf at all, such as a key a release has made
// required. It tries every contract key at its sample first, then adds keys
// one at a time while the readers keep accepting. A key the reader rejects on
// its own stays out, so an older reader's probe omits keys it doesn't know.
func (p *Prober) discoverContext(ctx context.Context, keys []Key) error {
	var pending []Key
	for _, key := range keys {
		if !slices.Contains(p.mergeKeys, key.Name) && key.Sample != "" {
			pending = append(pending, key)
		}
	}
	ok, err := p.acceptsScaffold(ctx, pending)
	if err != nil {
		return err
	}
	if ok {
		p.scaffold = pending
		return nil
	}
	for _, key := range pending {
		trial := append(slices.Clone(p.scaffold), key)
		accepted, trialErr := p.acceptsScaffold(ctx, trial)
		if trialErr != nil {
			return trialErr
		}
		if accepted {
			p.scaffold = trial
		}
	}
	return nil
}

func (p *Prober) acceptsScaffold(ctx context.Context, keys []Key) (bool, error) {
	if len(keys) == 0 {
		return true, nil
	}
	var conf strings.Builder
	conf.WriteString(p.filler(""))
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		conf.WriteString(key.Name + " = " + key.Sample + "\n")
		names = append(names, key.Name)
	}
	return p.acceptsConf(ctx, conf.String(), names)
}
