// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package signing

import (
	"bytes"
	"embed"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed units/*.tmpl
var units embed.FS

// Install copies a built binary and rendered service templates under home/.local.
// It doesn't activate services or change Git or agent identity configuration.
func Install(home, binary, sshAgent string, config Config) error {
	dir := filepath.Join(home, ".local/share/devtools/signing")
	for _, path := range []string{dir, filepath.Join(home, ".local/bin"), config.RunDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create install directory: %w", err)
		}
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return fmt.Errorf("read signing binary: %w", err)
	}
	//nolint:gosec // The installed command must be executable by its owner.
	if err = os.WriteFile(filepath.Join(home, ".local/bin/ssh-sign"), data, 0o700); err != nil {
		return fmt.Errorf("install signing binary: %w", err)
	}
	for _, name := range []string{"signing-agent.service", "dev.tbhb.devtools.signing.plist"} {
		rendered, renderErr := renderUnit(name, sshAgent, filepath.Join(config.RunDir, "agent.sock"))
		if renderErr != nil {
			return renderErr
		}
		if err = os.WriteFile(filepath.Join(dir, name), rendered, 0o600); err != nil {
			return fmt.Errorf("install signing unit: %w", err)
		}
	}
	return nil
}

func renderUnit(name, agent, socket string) ([]byte, error) {
	escape := func(value string) string {
		if strings.HasSuffix(name, ".plist") {
			var out bytes.Buffer
			if err := xml.EscapeText(&out, []byte(value)); err != nil {
				panic(err)
			} // bytes.Buffer writes can't fail.
			return out.String()
		}
		value = strings.ReplaceAll(value, "%", "%%")
		value = strings.ReplaceAll(value, "$", "$$")
		value = strings.ReplaceAll(value, "\\", "\\\\")
		return strings.ReplaceAll(value, "\"", "\\\"")
	}
	source, err := units.ReadFile("units/" + name + ".tmpl")
	if err != nil {
		return nil, fmt.Errorf("read unit template: %w", err)
	}
	parsed, err := template.New(name).Parse(string(source))
	if err != nil {
		return nil, fmt.Errorf("parse unit template: %w", err)
	}
	var out bytes.Buffer
	err = parsed.Execute(&out, struct{ Agent, Socket string }{escape(agent), escape(socket)})
	return out.Bytes(), err
}
