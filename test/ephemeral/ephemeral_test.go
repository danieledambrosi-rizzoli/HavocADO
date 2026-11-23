package ephemeral

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTempDir(t *testing.T) {
	dir, cleanup := TempDir(t, "test-*")
	defer cleanup()

	if dir == "" {
		t.Fatal("expected non-empty dir path")
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("failed to stat dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
}

func TestTempDirWithFiles(t *testing.T) {
	files := map[string]string{
		"file1.txt": "content1",
		"subdir/file2.txt": "content2",
	}

	dir, cleanup := TempDirWithFiles(t, "test-*", files)
	defer cleanup()

	for relPath, expectedContent := range files {
		fullPath := filepath.Join(dir, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatalf("failed to read %s: %v", relPath, err)
		}
		if string(content) != expectedContent {
			t.Errorf("file %s: expected %q, got %q", relPath, expectedContent, string(content))
		}
	}
}

func TestTempDirCleanup(t *testing.T) {
	var dirPath string

	func() {
		dir, cleanup := TempDir(t, "cleanup-test-*")
		dirPath = dir
		cleanup()
	}()

	// After cleanup, directory should not exist
	if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
		t.Errorf("expected temp dir to be cleaned up, but it still exists at %s", dirPath)
	}
}