// Package testmods builds the bundled modules for tests that need real
// module executables. It is only imported from _test files.
package testmods

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Build compiles the named bundled modules (e.g. "file", "command") into a
// new temporary directory and returns it. The caller removes the directory.
func Build(names ...string) (string, error) {
	_, self, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(self), "..", "..")
	dir, err := os.MkdirTemp("", "onecheck-testmods")
	if err != nil {
		return "", err
	}
	for _, name := range names {
		cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "onecheck-"+name), "./cmd/onecheck-"+name)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			os.RemoveAll(dir)
			return "", fmt.Errorf("build module %s: %w\n%s", name, err, out)
		}
	}
	return dir, nil
}
