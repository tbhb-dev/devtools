// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

// Package consumercontract holds the versioned contract between a devtools
// release and the declarative configuration its consumers keep. The contract
// lists the keys and values the release's readers accept, and the same file
// declares the transitions a consumer must make to adopt the release.
package consumercontract

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	// SchemaVersion is the only contract schema this reader understands.
	SchemaVersion = 1
	// ConfFile is the one configuration file schema 1 covers.
	ConfFile = ".agents/agent-skills.conf"
	// ContractPath locates the structured source in a release tree.
	ContractPath = ".devtools/consumer-contract.toml"
	// NotesPath locates the notes rendered from that source.
	NotesPath = "docs/consumer-transitions.md"
)

// Value classes a key may accept, and the transition kinds and conditions.
const (
	ClassEmpty    = "empty"
	ClassNonempty = "nonempty"
	ClassValue    = "value"

	KindRequiredKeyAdded     = "required-key-added"
	KindRequiredValueChanged = "required-value-changed"
	KindAcceptedRemoved      = "accepted-value-removed"

	WhenKeyAbsent     = "key-absent"
	WhenValueRejected = "value-rejected"
)

var (
	// ErrUnsupportedSchema reports a contract this reader can't interpret.
	ErrUnsupportedSchema = errors.New("unsupported consumer contract schema version")
	// ErrInvalidContract reports a contract that breaks a schema rule.
	ErrInvalidContract = errors.New("invalid consumer contract")

	tagForm = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

// Document is the contract and the transition declaration for one release.
type Document struct {
	SchemaVersion int `toml:"schema_version"`
	// Baseline marks the first release with a contract. Every later release
	// sets BaselineTag to that release, so an updater can tell a consumer
	// older than the baseline which tag to adopt first.
	Baseline    bool       `toml:"baseline"`
	BaselineTag string     `toml:"baseline_tag"`
	Keys        []Key      `toml:"key"`
	Transition  Transition `toml:"transition"`
}

// Key is one setting in a configuration file.
//
// Required means the readers reject the file when the key is absent.
// Accepts lists the value classes the readers accept once the key is
// present. A key with a Value accepts that exact string and no other.
// Sample is a value the readers accept, which the probes write.
type Key struct {
	File     string   `toml:"file"`
	Name     string   `toml:"name"`
	Required bool     `toml:"required"`
	Accepts  []string `toml:"accepts"`
	Value    string   `toml:"value"`
	Sample   string   `toml:"sample"`
}

// Transition is the release's explicit declaration: either None, or the
// entries a consumer must apply.
type Transition struct {
	None    bool    `toml:"none"`
	Entries []Entry `toml:"entry"`
}

// Entry declares one required consumer change. AfterRelease is the
// compatibility sequencing: the earliest source release whose reader
// accepts the target state.
type Entry struct {
	Kind         string `toml:"kind"`
	File         string `toml:"file"`
	Key          string `toml:"key"`
	AppliesWhen  string `toml:"applies_when"`
	TargetValue  string `toml:"target_value"`
	AfterRelease string `toml:"after_release"`
	Verify       string `toml:"verify"`
	Rollback     string `toml:"rollback"`
}

// Parse decodes and validates a contract. It rejects an unsupported schema
// version before it looks at anything else, and unknown fields after.
func Parse(data []byte) (*Document, error) {
	var head struct {
		SchemaVersion int `toml:"schema_version"`
	}
	if err := toml.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidContract, err)
	}
	if head.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf(
			"%w: %d (this reader supports %d)",
			ErrUnsupportedSchema,
			head.SchemaVersion,
			SchemaVersion,
		)
	}
	var doc Document
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidContract, err)
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
}

// Lookup returns the key with the given file and name.
func (d *Document) Lookup(file, name string) (Key, bool) {
	for _, key := range d.Keys {
		if key.File == file && key.Name == name {
			return key, true
		}
	}
	return Key{}, false
}

// Validate checks the schema rules that hold without running a reader.
func (d *Document) Validate() error {
	if len(d.Keys) == 0 {
		return invalid("the contract lists no keys")
	}
	switch {
	case d.Baseline && d.BaselineTag != "":
		return invalid("the baseline release cannot name a baseline tag")
	case !d.Baseline && !tagForm.MatchString(d.BaselineTag):
		return invalid("a release after the baseline needs baseline_tag as a release tag, got %q", d.BaselineTag)
	}
	seen := make(map[string]bool)
	for _, key := range d.Keys {
		if err := key.validate(); err != nil {
			return err
		}
		if id := key.ID(); seen[id] {
			return invalid("key %s is listed twice", id)
		}
		seen[key.ID()] = true
	}
	return d.validateTransition()
}

func (k Key) validate() error {
	if k.File != ConfFile {
		return invalid("key %q names unsupported file %q (schema 1 covers %s)", k.Name, k.File, ConfFile)
	}
	if k.Name == "" || strings.ContainsAny(k.Name, " \t=#") {
		return invalid("key name %q is empty or malformed", k.Name)
	}
	if len(k.Accepts) == 0 {
		return invalid("key %s accepts no value class", k.ID())
	}
	for index, class := range k.Accepts {
		if !slices.Contains([]string{ClassEmpty, ClassNonempty, ClassValue}, class) {
			return invalid("key %s accepts unknown class %q", k.ID(), class)
		}
		if slices.Contains(k.Accepts[:index], class) {
			return invalid("key %s lists class %q twice", k.ID(), class)
		}
	}
	return k.validateSample()
}

func (k Key) validateSample() error {
	hasValue := slices.Contains(k.Accepts, ClassValue)
	switch {
	case hasValue && slices.Contains(k.Accepts, ClassNonempty):
		return invalid("key %s accepts both an exact value and any nonempty value", k.ID())
	case hasValue != (k.Value != ""):
		return invalid("key %s must set value exactly when it accepts the value class", k.ID())
	case k.Sample == "":
		return invalid("key %s needs a sample value", k.ID())
	case !hasValue && !slices.Contains(k.Accepts, ClassNonempty):
		return invalid("key %s accepts no nonempty value, so it has no usable sample", k.ID())
	case hasValue && k.Sample != k.Value:
		return invalid("key %s sample must equal its value", k.ID())
	case strings.ContainsAny(k.Sample+k.Value, "\n\r"):
		return invalid("key %s holds a multi-line value", k.ID())
	}
	return nil
}

func (d *Document) validateTransition() error {
	t := d.Transition
	switch {
	case t.None && len(t.Entries) != 0:
		return invalid("the declaration is both none and a list of transitions")
	case !t.None && len(t.Entries) == 0:
		return invalid("declare transition.none = true or list every required transition")
	case d.Baseline && !t.None:
		return invalid("the baseline release must declare no transition")
	}
	for _, entry := range t.Entries {
		if err := entry.validate(d); err != nil {
			return err
		}
	}
	return nil
}

func (e Entry) validate(doc *Document) error {
	id := e.ID()
	if !slices.Contains([]string{KindRequiredKeyAdded, KindRequiredValueChanged, KindAcceptedRemoved}, e.Kind) {
		return invalid("transition %s has unknown kind", id)
	}
	if e.File != ConfFile || e.Key == "" {
		return invalid("transition %s must name %s and a key", id, ConfFile)
	}
	if !slices.Contains([]string{WhenKeyAbsent, WhenValueRejected}, e.AppliesWhen) {
		return invalid("transition %s has unknown applies_when %q", id, e.AppliesWhen)
	}
	if !tagForm.MatchString(e.AfterRelease) {
		return invalid("transition %s needs after_release as a release tag, got %q", id, e.AfterRelease)
	}
	if e.Verify == "" || e.Rollback == "" {
		return invalid("transition %s needs both a verification step and a rollback action", id)
	}
	key, kept := doc.Lookup(e.File, e.Key)
	if !kept {
		if e.TargetValue != "" {
			return invalid("transition %s removes its key, so it takes no target value", id)
		}
		return nil
	}
	if !key.AcceptsValue(e.TargetValue) {
		return invalid("transition %s targets a value its own contract does not accept", id)
	}
	return nil
}

// ID names a key as file:name.
func (k Key) ID() string { return k.File + ":" + k.Name }

// ID names an entry as kind file:key.
func (e Entry) ID() string { return e.Kind + " " + e.File + ":" + e.Key }

// AcceptsValue says whether the contract accepts a present value.
func (k Key) AcceptsValue(value string) bool {
	switch {
	case value == "":
		return slices.Contains(k.Accepts, ClassEmpty)
	case slices.Contains(k.Accepts, ClassValue):
		return value == k.Value
	default:
		return slices.Contains(k.Accepts, ClassNonempty)
	}
}

// States lists every state of the key the contract accepts: "absent" when
// the key is optional, then each accepted class.
func (k Key) States() StateSet {
	states := StateSet{}
	if !k.Required {
		states.Add(StateAbsent)
	}
	if slices.Contains(k.Accepts, ClassEmpty) {
		states.Add(StateEmpty)
	}
	if slices.Contains(k.Accepts, ClassNonempty) {
		states.Add(StateNonempty)
	}
	if slices.Contains(k.Accepts, ClassValue) {
		states.Add(ValueState(k.Value))
	}
	return states
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidContract, fmt.Sprintf(format, args...))
}
