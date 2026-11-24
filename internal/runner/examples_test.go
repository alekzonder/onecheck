package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/onecheck/internal/config"
	"github.com/alekzonder/onecheck/internal/testmods"
)

// bundledModules lists the modules built from cmd/onecheck-<name>.
func bundledModules(t *testing.T) []string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join("..", "..", "cmd", "onecheck-*"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range dirs {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			names = append(names, strings.TrimPrefix(filepath.Base(d), "onecheck-"))
		}
	}
	if len(names) == 0 {
		t.Fatal("no bundled modules found under cmd/")
	}
	return names
}

// The example config shipped in the repository must load and validate, so
// that documentation cannot drift away from the code, and must show every
// bundled module at least once.
func TestExamplesLoadAndValidate(t *testing.T) {
	t.Setenv("API_TOKEN", "example-token")
	cfg, err := config.Load(filepath.Join("..", "..", "examples", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	names := bundledModules(t)
	used := map[string]bool{}
	for _, r := range cfg.Rules {
		used[r.Module] = true
	}
	for _, name := range names {
		if !used[name] {
			t.Errorf("module %s has no example rule in examples/", name)
		}
	}

	dir, err := testmods.Build(names...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	cfg.Paths.Modules = dir
	for _, err := range Validate(context.Background(), cfg) {
		t.Error(err)
	}
}
