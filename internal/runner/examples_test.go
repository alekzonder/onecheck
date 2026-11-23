package runner

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alekzonder/onecheck/internal/config"
)

// The example config shipped in the repository must load and validate, so
// that documentation cannot drift away from the code.
func TestExamplesLoadAndValidate(t *testing.T) {
	t.Setenv("API_TOKEN", "example-token")
	cfg, err := config.Load(filepath.Join("..", "..", "examples", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 6 {
		t.Errorf("want 6 rules (3 in config.yaml, 3 in conf.d/web.yaml), got %d", len(cfg.Rules))
	}
	cfg.Paths.Modules = modDir
	for _, err := range Validate(context.Background(), cfg) {
		t.Error(err)
	}
}
