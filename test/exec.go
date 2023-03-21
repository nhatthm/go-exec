// Package test provides helpers to test the exec package.
package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func prepareBinary(path, content string) error {
	content = fmt.Sprintf("#!/usr/bin/env bash\n%s", content)

	return os.WriteFile(filepath.Clean(path), []byte(content), 0o755) //nolint: gosec,wrapcheck,gomnd
}

// Test creates a test case that will prepare a binary in a temporary directory.
func Test(binaryName, binaryContent string, f func(t *testing.T)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		tmpDir := t.TempDir()
		t.Setenv("PATH", fmt.Sprintf("%s:/usr/bin:/bin", tmpDir))

		err := prepareBinary(filepath.Join(tmpDir, binaryName), binaryContent)
		require.NoError(t, err)

		f(t)
	}
}
