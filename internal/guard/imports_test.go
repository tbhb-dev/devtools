// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package guard_test

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAC10Imports(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	var inspected int
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		inspected++
		file, parseErr := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		require.NoError(t, parseErr)
		for _, spec := range file.Imports {
			path, unquoteErr := strconv.Unquote(spec.Path.Value)
			require.NoError(t, unquoteErr)
			assert.Contains(t, []string{"fmt", "regexp", "strings"}, path, "%s imports %s", name, path)
		}
	}
	assert.Positive(t, inspected)
}
