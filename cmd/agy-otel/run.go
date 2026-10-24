// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	maxPayload = 4 * 1024 * 1024
	recordMode = 0o600
)

type payload struct {
	Session    *string `json:"conversationId"`
	Root       *string `json:"rootConversationId"`
	Parent     *string `json:"parentConversationId"`
	Model      *string `json:"modelName"`
	Agent      *string `json:"agentName"`
	Exec       *string `json:"executionId"`
	Step       *int64  `json:"stepIdx"`
	Invocation *int64  `json:"invocationNum"`
	Execution  *int64  `json:"executionNum"`
	ToolCall   *struct {
		Name *string         `json:"name"`
		Args json.RawMessage `json:"args"`
	} `json:"toolCall"`
	Result      json.RawMessage `json:"result"`
	Error       json.RawMessage `json:"error"`
	Termination *string         `json:"terminationReason"`
	Idle        *bool           `json:"fullyIdle"`
}

type record struct {
	Version     int     `json:"v"`
	Event       string  `json:"event"`
	Timestamp   int64   `json:"ts_ms"`
	Session     *string `json:"session,omitempty"`
	Root        *string `json:"root,omitempty"`
	Parent      *string `json:"parent,omitempty"`
	Model       *string `json:"model,omitempty"`
	Agent       *string `json:"agent,omitempty"`
	Exec        *string `json:"exec,omitempty"`
	Step        *int64  `json:"step,omitempty"`
	Invocation  *int64  `json:"invocation,omitempty"`
	Execution   *int64  `json:"execution,omitempty"`
	Tool        *string `json:"tool,omitempty"`
	ArgsBytes   *int    `json:"args_bytes,omitempty"`
	ResultBytes *int    `json:"result_bytes,omitempty"`
	Error       *bool   `json:"error,omitempty"`
	Termination *string `json:"termination,omitempty"`
	Idle        *bool   `json:"idle,omitempty"`
}

// Run deliberately has no output or failure status: hooks must never influence agy.
func Run(args []string, in io.Reader) {
	runHook(args, in, stateHome(), func(name string, flag int, perm os.FileMode) (spoolFile, error) {
		return os.OpenFile(name, flag, perm)
	})
}

func stateHome() string {
	if state := os.Getenv("XDG_STATE_HOME"); state != "" {
		return state
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state")
}

func runHook(args []string, in io.Reader, state string, open openFile) {
	received := time.Now()
	if len(args) != 2 || args[0] != "hook" || !knownEvent(args[1]) || state == "" {
		return
	}
	data, err := io.ReadAll(io.LimitReader(in, maxPayload+1))
	if err != nil || len(data) > maxPayload {
		return
	}
	var input payload
	if json.Unmarshal(data, &input) != nil {
		return
	}
	entry := input.record(args[1], received.UnixMilli())
	encoded, err := json.Marshal(entry)
	if err != nil {
		return
	}
	writeRecord(state, encoded, received, open)
}

func knownEvent(event string) bool {
	switch event {
	case "SessionStart", "PreToolUse", "PostToolUse", "PreInvocation", "PostInvocation", "Stop":
		return true
	default:
		return false
	}
}

func hashID(id *string) *string {
	if id == nil {
		return nil
	}
	digest := sha256.Sum256([]byte(*id))
	value := "sha-256-64:" + hex.EncodeToString(digest[:8])
	return &value
}

// Strings count decoded UTF-8 bytes. Structured values count their JSON bytes.
func valueBytes(value json.RawMessage) *int {
	if len(value) == 0 {
		return nil
	}
	size := len(value)
	var decoded string
	if json.Unmarshal(value, &decoded) == nil && string(value) != "null" {
		size = len(decoded)
	}
	return &size
}

func (p payload) record(event string, timestamp int64) record {
	r := record{
		Version: 1, Event: event, Timestamp: timestamp,
		Session: hashID(p.Session), Root: hashID(p.Root), Parent: hashID(p.Parent),
		Model: p.Model, Agent: p.Agent, Exec: p.Exec,
		Step: p.Step, Invocation: p.Invocation, Execution: p.Execution,
		ResultBytes: valueBytes(p.Result), Termination: p.Termination, Idle: p.Idle,
	}
	if p.ToolCall != nil {
		r.Tool = p.ToolCall.Name
		r.ArgsBytes = valueBytes(p.ToolCall.Args)
	}
	if len(p.Error) != 0 {
		set := string(p.Error) != "null"
		r.Error = &set
	}
	return r
}

type spoolFile interface {
	io.Writer
	Sync() error
	Close() error
}

type openFile func(name string, flag int, perm os.FileMode) (spoolFile, error)

func writeRecord(state string, data []byte, received time.Time, open openFile) {
	root := filepath.Join(state, "agy-otel")
	for _, dir := range []string{root, filepath.Join(root, "tmp"), filepath.Join(root, "new")} {
		if os.MkdirAll(dir, 0o700) != nil {
			return
		}
	}
	name := fmt.Sprintf("%d.%d.%s.json", received.UnixNano(), os.Getpid(), rand.Text())
	tmp := filepath.Join(root, "tmp", name)
	file, err := open(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, recordMode)
	if err != nil {
		return
	}
	// Cleanup touches only this call's unpublished file. Rename removes it on success.
	defer func() { _ = os.Remove(tmp) }()
	if !save(file, data) {
		return
	}
	if os.Rename(tmp, filepath.Join(root, "new", name)) != nil {
		return
	}
}

func save(file spoolFile, data []byte) bool {
	n, err := file.Write(data)
	if err != nil || n != len(data) {
		_ = file.Close()
		return false
	}
	if file.Sync() != nil {
		_ = file.Close()
		return false
	}
	return file.Close() == nil
}
