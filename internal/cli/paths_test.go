package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// line returns the output line that starts with the label.
func line(t *testing.T, out, label string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(l); len(f) > 0 && f[0] == label {
			return l
		}
	}
	t.Fatalf("no %q line in:\n%s", label, out)
	return ""
}

func TestPathsPrintsEveryPath(t *testing.T) {
	dir, cfg := setup(t, "include: [conf.d/*.yaml]", passRule)
	inc := filepath.Join(dir, "conf.d", "extra.yaml")
	if err := os.MkdirAll(filepath.Dir(inc), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inc, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := execute("paths", "-c", cfg)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	want := map[string]string{
		"config":  cfg,
		"name":    "main",
		"include": inc,
		"log":     filepath.Join(dir, "log", "main", "onecheck.log"),
		"state":   filepath.Join(dir, "state", "main"),
		"results": filepath.Join(dir, "results", "main"),
		"events":  filepath.Join(dir, "events"),
		"modules": modDir,
	}
	for label, path := range want {
		if f := strings.Fields(line(t, out, label)); f[1] != path {
			t.Errorf("%s = %q, want %q", label, f[1], path)
		}
	}
	if !strings.Contains(line(t, out, "config"), "--config") {
		t.Errorf("config line does not say how the config was found:\n%s", out)
	}
	if bin := strings.Fields(line(t, out, "binary"))[1]; !filepath.IsAbs(bin) {
		t.Errorf("binary path %q is not absolute", bin)
	}
}

func TestPathsMarksMissingPathsOnly(t *testing.T) {
	_, cfg := setup(t, "", passRule)
	_, before, _ := execute("paths", "-c", cfg)
	for _, label := range []string{"log", "state", "results", "events"} {
		if !strings.Contains(line(t, before, label), "(missing)") {
			t.Errorf("%s is not marked missing before the first run:\n%s", label, before)
		}
	}
	for _, label := range []string{"config", "modules", "binary"} {
		if strings.Contains(line(t, before, label), "(missing)") {
			t.Errorf("%s exists but is marked missing:\n%s", label, before)
		}
	}
	if code, _, errOut := execute("-c", cfg); code != 0 {
		t.Fatalf("run failed: %s", errOut)
	}
	_, after, _ := execute("paths", "-c", cfg)
	for _, label := range []string{"log", "state", "results"} {
		if strings.Contains(line(t, after, label), "(missing)") {
			t.Errorf("%s is still marked missing after a run:\n%s", label, after)
		}
	}
}

func TestPathsSaysConfigCameFromEnv(t *testing.T) {
	_, cfg := setup(t, "", passRule)
	t.Setenv("ONECHECK_CONFIG", cfg)
	_, out, _ := execute("paths")
	if !strings.Contains(line(t, out, "config"), "ONECHECK_CONFIG") {
		t.Errorf("config line does not name ONECHECK_CONFIG:\n%s", out)
	}
}

func TestPathsCreatesNothing(t *testing.T) {
	dir, cfg := setup(t, "", passRule)
	execute("paths", "-c", cfg)
	execute("paths", "-c", cfg, "--json")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "config.yaml" && e.Name() != "present.txt" {
			t.Errorf("paths created %s", e.Name())
		}
	}
}

func TestPathsWorksWithoutModulesAndWithBrokenRules(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	content := "name: main\npaths: {modules: nowhere}\nrules:\n  - {name: r, module: no-such-module}\n  - {name: bad}\n"
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := execute("paths", "-c", cfg)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if l := line(t, out, "modules"); !strings.Contains(l, filepath.Join(dir, "nowhere")) || !strings.Contains(l, "(missing)") {
		t.Errorf("modules line = %q", l)
	}
}

func TestPathsWithUnloadableConfigStillShowsItsPath(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfg, []byte("name: main\nno_such_key: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := execute("paths", "-c", cfg)
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if f := strings.Fields(line(t, out, "config")); f[1] != cfg {
		t.Errorf("config path not printed: %q", out)
	}
	if !strings.Contains(errOut, "no_such_key") {
		t.Errorf("error does not explain the problem: %q", errOut)
	}
}

func TestPathsJSON(t *testing.T) {
	dir, cfg := setup(t, "include: [conf.d/*.yaml]", passRule)
	inc := filepath.Join(dir, "conf.d", "extra.yaml")
	if err := os.MkdirAll(filepath.Dir(inc), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inc, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := execute("paths", "-c", cfg, "--json")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	type entry struct {
		Path   string `json:"path"`
		Exists bool   `json:"exists"`
		Source string `json:"source"`
	}
	var got struct {
		Name     string  `json:"name"`
		Config   entry   `json:"config"`
		Includes []entry `json:"includes"`
		Log      entry   `json:"log"`
		State    entry   `json:"state"`
		Results  entry   `json:"results"`
		Events   entry   `json:"events"`
		Modules  entry   `json:"modules"`
		Binary   entry   `json:"binary"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if got.Name != "main" || got.Config != (entry{cfg, true, "--config"}) {
		t.Errorf("name/config = %q %+v", got.Name, got.Config)
	}
	if len(got.Includes) != 1 || got.Includes[0] != (entry{Path: inc, Exists: true}) {
		t.Errorf("includes = %+v", got.Includes)
	}
	if got.Events != (entry{Path: filepath.Join(dir, "events")}) {
		t.Errorf("events = %+v", got.Events)
	}
	if got.Modules != (entry{Path: modDir, Exists: true}) || !got.Binary.Exists {
		t.Errorf("modules = %+v binary = %+v", got.Modules, got.Binary)
	}
	if got.Log.Path != filepath.Join(dir, "log", "main", "onecheck.log") ||
		got.State.Path != filepath.Join(dir, "state", "main") ||
		got.Results.Path != filepath.Join(dir, "results", "main") {
		t.Errorf("log/state/results = %+v %+v %+v", got.Log, got.State, got.Results)
	}
}
