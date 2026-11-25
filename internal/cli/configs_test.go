package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupDir writes one config file per entry of configs (file name -> rules)
// into a fresh directory. Each config is named after its file and shares the
// state, results, events and log directories of the directory.
func setupDir(t *testing.T, configs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "present.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	for file, rules := range configs {
		name := strings.TrimSuffix(strings.TrimSuffix(file, ".yaml"), ".yml")
		content := fmt.Sprintf("name: %s\npaths: {modules: %s, results: results, state: state, events: events}\nlog: {dir: log}\nrules:\n%s",
			name, modDir, strings.ReplaceAll(rules, "DIR", dir))
		writeFile(t, filepath.Join(dir, file), content)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const otherPassRule = "  - {name: other, module: file, params: {path: DIR/present.txt}, check: {exists: true}}\n"

func TestRunDir(t *testing.T) {
	dir := setupDir(t, map[string]string{"a.yaml": passRule, "b.yml": failRule})
	code, out, errOut := execute("-c", dir)
	if code != 1 {
		t.Errorf("code=%d err=%q", code, errOut)
	}
	a, b := strings.Index(out, "== a  "+filepath.Join(dir, "a.yaml")), strings.Index(out, "== b  "+filepath.Join(dir, "b.yml"))
	if a < 0 || b < a || !strings.Contains(out, "2 rules: 1 passed, 1 failed, 0 errors\n") {
		t.Errorf("want both configs in name order and one summary:\n%s", out)
	}
	for _, name := range []string{"a", "b"} {
		if _, err := os.Stat(filepath.Join(dir, "results", name)); err != nil {
			t.Errorf("config %s must write its own results: %v", name, err)
		}
	}
}

func TestRunDirBrokenConfig(t *testing.T) {
	dir := setupDir(t, map[string]string{"a.yaml": passRule})
	bad := filepath.Join(dir, "bad.yaml")
	writeFile(t, bad, "rules: []\n")
	code, out, errOut := execute("-c", dir)
	if code != 2 || !strings.Contains(out, "PASS") || !strings.Contains(out, "1 of 2 configs could not run") {
		t.Errorf("a broken config must not stop the others: code=%d out=%q", code, out)
	}
	if !strings.Contains(errOut, "onecheck: "+bad) || !strings.Contains(out, "== "+bad+"\nERROR  config:") {
		t.Errorf("the broken config must be named: out=%q err=%q", out, errOut)
	}
	_, out, _ = execute("-c", dir, "-o", "json")
	var doc struct {
		Configs []struct{ Name, Path, Error string }
		Results []struct{ Rule string }
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout must be pure JSON: %v\n%s", err, out)
	}
	if len(doc.Configs) != 2 || doc.Configs[1].Path != bad || doc.Configs[1].Error == "" || len(doc.Results) != 1 {
		t.Errorf("doc = %+v", doc)
	}
}

func TestRunDirDuplicateNames(t *testing.T) {
	dir := setupDir(t, map[string]string{"a.yaml": passRule})
	data, err := os.ReadFile(filepath.Join(dir, "a.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "copy.yaml"), string(data))
	code, out, errOut := execute("-c", dir)
	if code != 2 || strings.Contains(out, "PASS") || strings.Count(errOut, `name "a" is also used by`) != 2 {
		t.Errorf("configs sharing a name must both be errors: code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestRunDirRuleFilter(t *testing.T) {
	dir := setupDir(t, map[string]string{"a.yaml": passRule, "b.yaml": otherPassRule + failRule})
	code, out, errOut := execute("-c", dir, "--rule", "ok")
	if code != 0 || !strings.Contains(out, "1 rules: 1 passed") || strings.Contains(out, "== b") {
		t.Errorf("--rule ok must not run the rules of b: code=%d out=%q err=%q", code, out, errOut)
	}
	code, out, errOut = execute("-c", dir, "--rule", "ok", "--rule", "other")
	if code != 0 || !strings.Contains(out, "2 rules: 2 passed") {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
	dir = setupDir(t, map[string]string{"a.yaml": passRule, "b.yaml": otherPassRule})
	code, out, errOut = execute("-c", dir, "--rule", "nope")
	if code != 2 || out != "" || !strings.Contains(errOut, `no rule named "nope" in `+dir) {
		t.Errorf("unknown rule: code=%d out=%q err=%q", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "results")); err == nil {
		t.Error("an unknown rule must stop the run before anything runs")
	}
}

func TestRunDefaultDir(t *testing.T) {
	system, _ := noDefaultConfigs(t)
	src := setupDir(t, map[string]string{"a.yaml": passRule, "b.yaml": otherPassRule})
	for _, f := range []string{"a.yaml", "b.yaml"} {
		data, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Fatal(err)
		}
		// Relative paths would resolve against the system dir; make them absolute.
		content := strings.NewReplacer("results: results", "results: "+filepath.Join(src, "results"),
			"state: state", "state: "+filepath.Join(src, "state"), "events: events", "events: "+filepath.Join(src, "events"),
			"dir: log", "dir: "+filepath.Join(src, "log")).Replace(string(data))
		writeFile(t, filepath.Join(system, f), content)
	}
	code, out, errOut := execute()
	if code != 0 || !strings.Contains(out, "== a  ") || !strings.Contains(out, "== b  ") || !strings.Contains(out, "2 rules: 2 passed") {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestRunFileModeUnchanged(t *testing.T) {
	_, cfg := setup(t, "", passRule)
	_, out, _ := execute("-c", cfg)
	if strings.Contains(out, "==") || strings.Contains(out, "configs") {
		t.Errorf("a single config file must print as before:\n%s", out)
	}
}

func TestValidateDir(t *testing.T) {
	dir := setupDir(t, map[string]string{"a.yaml": passRule, "b.yaml": "  - {name: x, module: ghost}\n"})
	code, out, errOut := execute("validate", "-c", dir)
	if code != 2 || !strings.Contains(out, "config OK: 1 rules ("+filepath.Join(dir, "a.yaml")+")") {
		t.Errorf("code=%d out=%q", code, out)
	}
	if !strings.Contains(errOut, filepath.Join(dir, "b.yaml")+": rule x") || !strings.Contains(errOut, "1 of 2 configs") {
		t.Errorf("err=%q", errOut)
	}
}

func TestPathsDir(t *testing.T) {
	dir := setupDir(t, map[string]string{"a.yaml": passRule, "b.yaml": passRule})
	writeFile(t, filepath.Join(dir, "bad.yaml"), "bogus: 1\n")
	code, out, errOut := execute("paths", "-c", dir)
	if code != 2 || strings.Count("\n"+out, "\nconfig ") != 3 || strings.Count(out, "binary ") != 1 {
		t.Errorf("want 3 config blocks and one binary line: code=%d out=\n%s\nerr=%q", code, out, errOut)
	}
	if !strings.Contains(out, "name      a") || !strings.Contains(errOut, "bogus") {
		t.Errorf("out=%s err=%q", out, errOut)
	}
	_, out, _ = execute("paths", "-c", dir, "--json")
	var doc struct {
		Configs []struct {
			Name   string
			Config struct{ Path string }
		}
		Binary struct{ Path string }
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(doc.Configs) != 3 || doc.Configs[0].Name != "a" || doc.Configs[2].Config.Path != filepath.Join(dir, "bad.yaml") || doc.Binary.Path == "" {
		t.Errorf("doc = %+v", doc)
	}
}

func TestLogrotateDir(t *testing.T) {
	dir := setupDir(t, map[string]string{"a.yaml": passRule, "b.yaml": passRule})
	code, out, errOut := execute("logrotate", "-c", dir)
	if code != 0 || strings.Count(out, "logs: ") != 2 || strings.Count(out, "events: ") != 1 {
		t.Errorf("logs of both configs, the shared event queue once: code=%d out=%q err=%q", code, out, errOut)
	}
	for _, name := range []string{"a", "b"} {
		if !strings.Contains(out, filepath.Join(dir, "log", name, "onecheck.log")) {
			t.Errorf("no log line for %s:\n%s", name, out)
		}
	}
}
