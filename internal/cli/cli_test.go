package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/onecheck/internal/testmods"
)

var modDir string

func TestMain(m *testing.M) {
	dir, err := testmods.Build("file", "command")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	modDir = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// setup writes a config with the given extra top-level YAML and rules, and
// returns its directory and path.
func setup(t *testing.T, extra, rules string) (dir, cfg string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "present.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("name: main\npaths: {modules: %s, results: results, state: state, events: events}\nlog: {dir: log}\n%s\nrules:\n%s",
		modDir, extra, strings.ReplaceAll(rules, "DIR", dir))
	cfg = filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, cfg
}

func execute(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Execute(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

const passRule = "  - {name: ok, module: file, params: {path: DIR/present.txt}, check: {exists: true}}\n"
const failRule = "  - {name: broken, module: file, params: {path: DIR/missing.txt}, check: {exists: true}}\n"

func TestRunDefaultCommand(t *testing.T) {
	_, cfg := setup(t, "", passRule)
	code, out, errOut := execute("-c", cfg)
	if code != 0 || !strings.Contains(out, "PASS") || !strings.Contains(out, "1 rules: 1 passed") {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
	code2, out2, _ := execute("run", "--config", cfg)
	if code2 != 0 || !strings.Contains(out2, "1 rules: 1 passed") {
		t.Errorf("explicit run: code=%d out=%q", code2, out2)
	}
}

func TestRunConfigFromEnv(t *testing.T) {
	_, cfg := setup(t, "", passRule)
	t.Setenv("ONECHECK_CONFIG", cfg)
	if code, out, errOut := execute(); code != 0 || !strings.Contains(out, "1 passed") {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestRunExitCodes(t *testing.T) {
	_, cfg := setup(t, "", passRule+failRule)
	if code, out, _ := execute("-c", cfg); code != 1 || !strings.Contains(out, "FAIL") {
		t.Errorf("failing rule: code=%d out=%q", code, out)
	}
	code, out, errOut := execute("-c", filepath.Join(t.TempDir(), "nope.yaml"))
	if code != 2 || out != "" || !strings.Contains(errOut, "onecheck:") {
		t.Errorf("missing config: code=%d out=%q err=%q", code, out, errOut)
	}
	_, bad := setup(t, "bogus_key: 1", passRule)
	if code, _, errOut := execute("-c", bad); code != 2 || !strings.Contains(errOut, "bogus_key") {
		t.Errorf("invalid config: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := execute("-c", cfg, "--rule", "nope"); code != 2 || !strings.Contains(errOut, "nope") {
		t.Errorf("unknown rule: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := execute("-c", cfg, "--no-such-flag"); code != 2 || !strings.Contains(errOut, "--help") {
		t.Errorf("unknown flag: code=%d err=%q", code, errOut)
	}
	if code, _, _ := execute("-c", cfg, "-o", "xml"); code != 2 {
		t.Errorf("unknown output format: code=%d", code)
	}
}

func TestRunJSONOnlyFailed(t *testing.T) {
	_, cfg := setup(t, "", passRule+failRule)
	code, out, _ := execute("-c", cfg, "-o", "json", "--only-failed")
	if code != 1 {
		t.Errorf("code = %d", code)
	}
	var doc struct {
		Summary struct{ Total, Passed, Failed int }
		Results []struct{ Rule, Status string }
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout must be pure JSON: %v\n%s", err, out)
	}
	if doc.Summary.Total != 2 || doc.Summary.Passed != 1 || len(doc.Results) != 1 || doc.Results[0].Rule != "broken" {
		t.Errorf("doc = %+v", doc)
	}
}

func TestRunRuleAndTagFlags(t *testing.T) {
	rules := passRule + failRule + "  - {name: tagged, module: file, tags: [t1], params: {path: DIR/present.txt}, check: {exists: true}}\n"
	_, cfg := setup(t, "", rules)
	code, out, _ := execute("-c", cfg, "--rule", "ok", "--tag", "t1")
	if code != 0 || !strings.Contains(out, "2 rules: 2 passed") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestValidateOKAndErrors(t *testing.T) {
	_, cfg := setup(t, "", passRule+failRule)
	code, out, _ := execute("validate", "-c", cfg)
	if code != 0 || !strings.Contains(out, "config OK: 2 rules") {
		t.Errorf("code=%d out=%q", code, out)
	}
	dir, bad := setup(t, "", passRule+"  - {name: a, module: ghost}\n  - {name: b, module: file, params: {path: /x}, check: {exists: {nope: 1}}}\n")
	code, _, errOut := execute("validate", "-c", bad)
	if code != 2 || !strings.Contains(errOut, "rule a") || !strings.Contains(errOut, "rule b") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "results")); err == nil {
		t.Error("validate must not run checks or create result directories")
	}
}

func TestLogLevelFlagOverridesConfig(t *testing.T) {
	dir, cfg := setup(t, "", passRule)
	// setup() already wrote "log: {dir: log}"; rewrite with level error.
	data, _ := os.ReadFile(cfg)
	if err := os.WriteFile(cfg, bytes.Replace(data, []byte("log: {dir: log}"), []byte("log: {dir: log, level: error}"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(dir, "log", "main", "onecheck.log")

	execute("-c", cfg)
	if data, _ := os.ReadFile(logFile); strings.Contains(string(data), "check passed") {
		t.Errorf("level error from config must hide info lines:\n%s", data)
	}
	execute("-c", cfg, "--log-level", "info")
	if data, _ := os.ReadFile(logFile); !strings.Contains(string(data), "check passed") {
		t.Errorf("--log-level info must override the config:\n%s", data)
	}
	if code, _, errOut := execute("-c", cfg, "--log-level", "loud"); code != 2 || !strings.Contains(errOut, "loud") {
		t.Errorf("bad level: code=%d err=%q", code, errOut)
	}
}

func TestVersion(t *testing.T) {
	code, out, _ := execute("version")
	if code != 0 || !strings.HasPrefix(out, "onecheck ") || !strings.Contains(out, "protocol 1") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestHelpListsCommands(t *testing.T) {
	code, out, _ := execute("--help")
	if code != 0 {
		t.Errorf("code = %d", code)
	}
	for _, want := range []string{"run", "validate", "paths", "version", "--config", "--log-level", "--only-failed", "Exit codes", "/etc/onecheck/config.yaml"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
	if code, _, errOut := execute("frobnicate"); code != 2 || !strings.Contains(errOut, "frobnicate") {
		t.Errorf("unknown command: code=%d err=%q", code, errOut)
	}
}

func TestLogrotateCommand(t *testing.T) {
	dir, cfg := setup(t, "events: {ttl: 1h}", failRule)
	data, _ := os.ReadFile(cfg)
	data = bytes.Replace(data, []byte("log: {dir: log}"), []byte("log: {dir: log, rotate: {max_size: 10B, keep: 2, compress: false}}"), 1)
	if err := os.WriteFile(cfg, data, 0o644); err != nil {
		t.Fatal(err)
	}
	execute("-c", cfg) // writes a log and one "fail" event
	events, _ := filepath.Glob(filepath.Join(dir, "events", "*.json"))
	if len(events) != 1 {
		t.Fatalf("events = %v", events)
	}
	expired := filepath.Join(dir, "events", "old.json")
	if err := os.WriteFile(expired, []byte(`{"expires_at":"2020-01-01T00:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(dir, "log", "main", "onecheck.log")

	code, out, errOut := execute("logrotate", "-c", cfg, "--events")
	if code != 0 || !strings.Contains(out, "events: removed 1") {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Error("the expired event must be removed")
	}
	if _, err := os.Stat(logFile + ".1"); err == nil {
		t.Error("--events must not rotate logs")
	}

	code, out, errOut = execute("logrotate", "-c", cfg)
	if code != 0 || !strings.Contains(out, "logs: rotated") || !strings.Contains(out, "events: removed 0") {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
	if _, err := os.Stat(logFile + ".1"); err != nil {
		t.Errorf("the log must be rotated to onecheck.log.1: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "events", "*.json")); len(left) != 1 {
		t.Errorf("the fresh event must stay: %v", left)
	}
}

func TestModuleListCommand(t *testing.T) {
	_, cfg := setup(t, "", passRule)
	code, out, errOut := execute("module", "list", "-c", cfg)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	for _, want := range []string{"NAME", "command", "file", "yes", modDir} {
		if !strings.Contains(out, want) {
			t.Errorf("module list lacks %q:\n%s", want, out)
		}
	}
	code, out, _ = execute("module", "list", "--modules-dir", t.TempDir())
	if code != 0 || !strings.Contains(out, "no modules installed") {
		t.Errorf("empty dir without a config: code=%d out=%q", code, out)
	}
}

func TestLogLevelFlagAlsoLogsToStderr(t *testing.T) {
	_, cfg := setup(t, "", passRule)
	if _, _, errOut := execute("-c", cfg); strings.Contains(errOut, "check passed") {
		t.Errorf("without --log-level stderr must stay quiet:\n%s", errOut)
	}
	_, out, errOut := execute("-c", cfg, "--log-level", "debug")
	for _, want := range []string{"run started", "collecting fields", "fields collected", "check passed"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("--log-level debug: stderr lacks %q:\n%s", want, errOut)
		}
	}
	if strings.Contains(out, "level=") {
		t.Errorf("log lines leaked into stdout:\n%s", out)
	}
}
