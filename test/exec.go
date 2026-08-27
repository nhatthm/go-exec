// Package test provides helpers to test the exec package.
package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func prepareBinary(path, content string) error {
	content = "#!/usr/bin/env bash\n" + content

	return os.WriteFile(filepath.Clean(path), []byte(content), 0o755) //nolint: gosec,wrapcheck,mnd
}

// Test creates a test case that will prepare a binary in a temporary directory.
func Test(binaryName, binaryContent string, f func(t *testing.T)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		tmpDir := t.TempDir()
		t.Setenv("PATH", tmpDir+":/usr/bin:/bin")

		err := prepareBinary(filepath.Join(tmpDir, binaryName), binaryContent)
		require.NoError(t, err)

		f(t)
	}
}
