// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package releaseassets

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	_ "embed"
)

const (
	commandArgs    = 3
	executableMode = 0o755
)

const (
	metadataName = "release.json"
	checksumName = "SHA256SUMS"
)

//go:embed inventory.json
var inventoryJSON []byte

type inventory struct {
	Platforms []string `json:"platforms"`
	Binaries  []string `json:"binaries"`
}

type metadata struct {
	Version   string    `json:"version"`
	Commit    string    `json:"commit"`
	BuildDate string    `json:"build_date"`
	Inventory inventory `json:"inventory"`
}

var (
	errEvidence = errors.New("release evidence differs")
	errUsage    = errors.New("usage: release-assets build|publish TAG DIR; verify TAG COMMIT DATE DIR INVENTORY")
)

type command func(context.Context, []string, string, ...string) ([]byte, error)

func execute(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s %v: %w: %s", name, args, err, out)
	}
	return out, nil
}

func Run(ctx context.Context, args []string) error {
	if len(args) == commandArgs {
		switch args[0] {
		case "build":
			return build(ctx, execute, args[1], args[2])
		case "publish":
			return publish(ctx, execute, args[1], args[2])
		}
	}
	if len(args) == 6 && args[0] == "verify" {
		raw, err := os.ReadFile(args[5])
		if err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
		var inv inventory
		if err = json.Unmarshal(raw, &inv); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
		return verify(args[4], metadata{Version: args[1], Commit: args[2], BuildDate: args[3], Inventory: inv})
	}
	return errUsage
}

func archiveName(tag, platform string) string {
	ext := ".tar.gz"
	if strings.HasPrefix(platform, "windows/") {
		ext = ".zip"
	}
	return "devtools_" + tag + "_" + strings.ReplaceAll(platform, "/", "_") + ext
}

func executable(name, platform string) string {
	if strings.HasPrefix(platform, "windows/") {
		return name + ".exe"
	}
	return name
}

func releaseMetadata(ctx context.Context, cmd command, tag string) (metadata, error) {
	commit, err := cmd(ctx, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return metadata{}, fmt.Errorf("release metadata: %w", err)
	}
	tagged, err := cmd(ctx, nil, "git", "rev-parse", "refs/tags/"+tag+"^{commit}")
	if err != nil {
		return metadata{}, fmt.Errorf("release metadata: %w", err)
	}
	if !bytes.Equal(commit, tagged) {
		return metadata{}, fmt.Errorf("tag does not name HEAD: %w", errEvidence)
	}
	date, err := cmd(ctx, nil, "git", "show", "-s", "--format=%cI", "HEAD")
	if err != nil {
		return metadata{}, fmt.Errorf("release metadata: %w", err)
	}
	stamp, err := time.Parse(time.RFC3339, strings.TrimSpace(string(date)))
	if err != nil {
		return metadata{}, fmt.Errorf("release metadata: %w", err)
	}
	var inv inventory
	if err = json.Unmarshal(inventoryJSON, &inv); err != nil {
		return metadata{}, fmt.Errorf("release metadata: %w", err)
	}
	return metadata{tag, strings.TrimSpace(string(commit)), stamp.UTC().Format(time.RFC3339), inv}, nil
}

func build(ctx context.Context, cmd command, tag, dir string) error {
	meta, err := releaseMetadata(ctx, cmd, tag)
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	flags, err := cmd(ctx, nil, "bash", ".devtools/scripts/go-ldflags.sh")
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	staging, err := os.MkdirTemp("", "devtools-release-*")
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	defer os.RemoveAll(staging)
	for _, platform := range meta.Inventory.Platforms {
		if err = buildPlatform(
			ctx,
			cmd,
			staging,
			platform,
			strings.TrimSpace(string(flags)),
			meta.Inventory,
		); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
		if err = pack(dir, staging, platform, meta); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	if err = os.WriteFile(filepath.Join(dir, metadataName), append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	sums, err := checksums(dir, meta)
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	if err = os.WriteFile(filepath.Join(dir, checksumName), sums, 0o600); err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	return verify(dir, meta)
}

func buildPlatform(ctx context.Context, cmd command, staging, platform, flags string, inv inventory) error {
	parts := strings.Split(platform, "/")
	for _, bin := range inv.Binaries {
		_, err := cmd(
			ctx,
			[]string{"CGO_ENABLED=0", "GOOS=" + parts[0], "GOARCH=" + parts[1]},
			"go",
			"build",
			"-trimpath",
			"-buildvcs=false",
			"-ldflags",
			flags,
			"-o",
			filepath.Join(staging, executable(bin, platform)),
			"./cmd/"+bin,
		)
		if err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
	}
	return nil
}

func assetNames(meta metadata) []string {
	names := make([]string, 0, 1+len(meta.Inventory.Platforms))
	names = append(names, metadataName)
	for _, platform := range meta.Inventory.Platforms {
		names = append(names, archiveName(meta.Version, platform))
	}
	slices.Sort(names)
	return names
}

func checksums(dir string, meta metadata) ([]byte, error) {
	var sums strings.Builder
	for _, name := range assetNames(meta) {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("release assets: %w", err)
		}
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(data), name)
	}
	return []byte(sums.String()), nil
}

func pack(dir, staging, platform string, meta metadata) error {
	var buf bytes.Buffer
	if strings.HasPrefix(platform, "windows/") {
		if err := packZIP(&buf, staging, platform, meta); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
	} else {
		if err := packTar(&buf, staging, meta); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
	}
	return wrapClose(os.WriteFile(filepath.Join(dir, archiveName(meta.Version, platform)), buf.Bytes(), 0o600))
}

func packZIP(buf *bytes.Buffer, staging, platform string, meta metadata) error {
	archive := zip.NewWriter(buf)
	for _, bin := range meta.Inventory.Binaries {
		name := executable(bin, platform)
		data, err := os.ReadFile(filepath.Join(staging, name))
		if err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(executableMode)
		header.Modified, err = time.Parse(time.RFC3339, meta.BuildDate)
		if err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
		entry, err := archive.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
		if _, err = entry.Write(data); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
	}
	return wrapClose(archive.Close())
}

func packTar(buf *bytes.Buffer, staging string, meta metadata) error {
	compressed := gzip.NewWriter(buf)
	archive := tar.NewWriter(compressed)
	stamp, err := time.Parse(time.RFC3339, meta.BuildDate)
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	for _, bin := range meta.Inventory.Binaries {
		data, readErr := os.ReadFile(filepath.Join(staging, bin))
		if readErr != nil {
			return fmt.Errorf("release assets: %w", readErr)
		}
		if err = archive.WriteHeader(
			&tar.Header{Name: bin, Mode: executableMode, Size: int64(len(data)), ModTime: stamp},
		); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
		if _, err = archive.Write(data); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
	}
	if err = archive.Close(); err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	return wrapClose(compressed.Close())
}

func verify(dir string, expected metadata) error {
	raw, err := os.ReadFile(filepath.Join(dir, metadataName))
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	var actual metadata
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&actual); err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing metadata: %w", errEvidence)
	}
	if actual.Version != expected.Version || actual.Commit != expected.Commit ||
		actual.BuildDate != expected.BuildDate ||
		!slices.Equal(actual.Inventory.Platforms, expected.Inventory.Platforms) ||
		!slices.Equal(actual.Inventory.Binaries, expected.Inventory.Binaries) {
		return fmt.Errorf("metadata: %w", errEvidence)
	}
	sums, err := checksums(dir, expected)
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	published, err := os.ReadFile(filepath.Join(dir, checksumName))
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	if !bytes.Equal(sums, published) {
		return fmt.Errorf("checksums: %w", errEvidence)
	}
	for _, platform := range expected.Inventory.Platforms {
		if err = verifyArchive(dir, platform, expected); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
	}
	return nil
}

func verifyArchive(dir, platform string, meta metadata) error {
	raw, err := os.ReadFile(filepath.Join(dir, archiveName(meta.Version, platform)))
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	expected := make(map[string]bool)
	for _, bin := range meta.Inventory.Binaries {
		expected[executable(bin, platform)] = true
	}
	check := func(name string, mode os.FileMode, size int64) error {
		if !expected[name] || !mode.IsRegular() || mode.Perm()&0o111 == 0 || size == 0 {
			return fmt.Errorf("archive entry %s: %w", name, errEvidence)
		}
		delete(expected, name)
		return nil
	}
	if strings.HasPrefix(platform, "windows/") {
		err = verifyZIP(raw, check)
	} else {
		err = verifyTar(raw, check)
	}
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	if len(expected) != 0 {
		return fmt.Errorf("archive inventory %s: %w", platform, errEvidence)
	}
	return nil
}

func verifyZIP(raw []byte, check func(string, os.FileMode, int64) error) error {
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	for _, entry := range archive.File {
		if err = check(entry.Name, entry.Mode(), entry.FileInfo().Size()); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
		stream, openErr := entry.Open()
		if openErr != nil {
			return fmt.Errorf("release assets: %w", openErr)
		}
		err = consume(stream)
		closeErr := stream.Close()
		if err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
		if closeErr != nil {
			return fmt.Errorf("release assets: %w", closeErr)
		}
	}
	return nil
}

func verifyTar(raw []byte, check func(string, os.FileMode, int64) error) error {
	compressed, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	archive := tar.NewReader(compressed)
	for {
		header, nextErr := archive.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("release assets: %w", nextErr)
		}
		if header.Typeflag != tar.TypeReg {
			return fmt.Errorf("non-regular archive entry: %w", errEvidence)
		}
		if err = check(header.Name, header.FileInfo().Mode(), header.Size); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
		if err = consume(archive); err != nil {
			return fmt.Errorf("release assets: %w", err)
		}
	}
	// Read the compression footer too, so archive completion can't hide a bad checksum.
	if err = consume(compressed); err != nil {
		return fmt.Errorf("release assets: %w", err)
	}
	return wrapClose(compressed.Close())
}

// consume bounds decompression while reading through checksum trailers.
func consume(reader io.Reader) error {
	const maxBinarySize = 512 << 20
	size, err := io.Copy(io.Discard, io.LimitReader(reader, maxBinarySize+1))
	if err != nil {
		return fmt.Errorf("consume archive: %w", err)
	}
	if size > maxBinarySize {
		return fmt.Errorf("decompressed entry too large: %w", errEvidence)
	}
	return nil
}

func wrapClose(err error) error {
	if err != nil {
		return fmt.Errorf("release file operation: %w", err)
	}
	return nil
}
