package ephemeral

import (
	"os"
	"path/filepath"
	"testing"
)

// TempDir creates a temporary directory for testing that will be cleaned up after the test.
// The directory is created in the system temp directory with a unique prefix.
// It returns the path to the created directory and a cleanup function.
func TempDir(t *testing.T, prefix string) (string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cleanup := func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Logf("warning: failed to clean up temp dir %s: %v", dir, err)
		}
	}

	// Register cleanup to run after test completes
	t.Cleanup(cleanup)

	return dir, cleanup
}

// TempDirWithFiles creates a temporary directory with the given files pre-populated.
// Files is a map of relative paths to content.
// Returns the temp directory path and a cleanup function.
func TempDirWithFiles(t *testing.T, prefix string, files map[string]string) (string, func()) {
	t.Helper()

	dir, cleanup := TempDir(t, prefix)

	for relPath, content := range files {
		fullPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create parent dirs for %s: %v", relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", relPath, err)
		}
	}

	return dir, cleanup
}

// CopyDir copies a source directory to a new temporary directory.
// Returns the path to the new temp directory and a cleanup function.
func CopyDir(t *testing.T, srcDir, prefix string) (string, func()) {
	t.Helper()

	destDir, cleanup := TempDir(t, prefix)

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("failed to read source dir %s: %v", srcDir, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		destPath := filepath.Join(destDir, entry.Name())

		if entry.IsDir() {
			if err := copyDirRecursive(srcPath, destPath); err != nil {
				t.Fatalf("failed to copy dir %s: %v", entry.Name(), err)
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				t.Fatalf("failed to copy file %s: %v", entry.Name(), err)
			}
		}
	}

	return destDir, cleanup
}

func copyDirRecursive(src, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := copyDirRecursive(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0644)
}