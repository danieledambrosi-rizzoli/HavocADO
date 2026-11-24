package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// repoRoot is the absolute path to the repository root.
//
// The payload builder resolves the Demon source tree and toolchain paths
// relative to the process working directory (utils.GetTeamserverPath uses
// os.Getwd), so TestMain changes into the repository root once for the whole
// package. Subtests can then run in parallel without touching the cwd.
var repoRoot string

func TestMain(m *testing.M) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: resolve repository root: %v\n", err)
		os.Exit(1)
	}

	repoRoot = root
	if err := os.Chdir(root); err != nil {
		fmt.Fprintf(os.Stderr, "integration: change to repository root: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}
