package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := write(t, t.TempDir(), "config.yaml", "name: main\nrules:\n  - name: a\n    module: file\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "main" || cfg.Path != path {
		t.Errorf("name=%q path=%q", cfg.Name, cfg.Path)
	}
	if cfg.Log.Level != "info" || cfg.Log.Format != "text" {
		t.Errorf("log = %+v", cfg.Log)
	}
	r := cfg.Log.Rotate
	if r.MaxSize != 10<<20 || r.Keep != 5 || time.Duration(r.MaxAge) != 30*24*time.Hour || !r.Compress {
		t.Errorf("rotate = %+v", r)
	}
	if time.Duration(cfg.Events.TTL) != 24*time.Hour || cfg.Run.Parallel != 4 || cfg.Run.MaxFieldSize != 10<<20 {
		t.Errorf("events=%+v run=%+v", cfg.Events, cfg.Run)
	}
	if len(cfg.Modules.Repos) != 1 || cfg.Modules.Repos[0] != "alekzonder/onecheck" {
		t.Errorf("repos = %v", cfg.Modules.Repos)
	}
	rule := cfg.Rules[0]
	if !rule.Enabled || rule.Timeout != 30*time.Second || rule.FailAfter != 1 || rule.PassAfter != 1 || rule.Err != nil {
		t.Errorf("rule = %+v", rule)
	}
}

func TestLoadDefaultPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if os.Geteuid() == 0 {
		t.Skip("default paths differ for root")
	}
	cfg, err := Load(write(t, t.TempDir(), "config.yaml", "name: main\n"))
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(home, ".local/state/onecheck")
	if cfg.Paths.Results != state+"/results" || cfg.Paths.Events != state+"/events" || cfg.Paths.State != state+"/state" {
		t.Errorf("paths = %+v", cfg.Paths)
	}
	if cfg.Paths.Modules != filepath.Join(home, ".local/lib/onecheck/modules") || cfg.Log.Dir != state+"/log" {
		t.Errorf("modules=%q log=%q", cfg.Paths.Modules, cfg.Log.Dir)
	}
	if cfg.ResultsDir() != state+"/results/main" || cfg.StateDir() != state+"/state/main" || cfg.LogFile() != state+"/log/main/onecheck.log" {
		t.Errorf("derived dirs: %q %q %q", cfg.ResultsDir(), cfg.StateDir(), cfg.LogFile())
	}
}

const fullConfig = `
name: web.prod-1
log:
  level: debug
  dir: /tmp/oc/log
  format: json
  rotate: {max_size: 1MB, keep: 2, max_age: 7d, compress: false}
paths:
  modules: /tmp/oc/modules
  results: rel/results
  events: /tmp/oc/events
  state: /tmp/oc/state
events: {ttl: 2h}
run: {parallel: 8, max_field_size: 1KB}
defaults: {timeout: 5s, fail_after: 3, pass_after: 2}
modules: {repos: [a/b, c/d]}
rules:
  - name: api
    module: http
    tags: [api, prod]
    timeout: 1500ms
    fail_after: 1
    params: {url: "https://example.com", headers: {X-A: b}}
    check: {status: 200}
  - name: off
    module: file
    enabled: false
`

func TestLoadFull(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(write(t, dir, "c.yaml", fullConfig))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "web.prod-1" || cfg.Log.Level != "debug" || cfg.Log.Format != "json" {
		t.Errorf("cfg = %+v", cfg)
	}
	if cfg.Log.Rotate.MaxSize != 1<<20 || cfg.Log.Rotate.Keep != 2 || cfg.Log.Rotate.Compress {
		t.Errorf("rotate = %+v", cfg.Log.Rotate)
	}
	if cfg.Paths.Results != filepath.Join(dir, "rel/results") {
		t.Errorf("relative path must resolve against the config dir, got %q", cfg.Paths.Results)
	}
	if cfg.Run.Parallel != 8 || cfg.Run.MaxFieldSize != 1024 || time.Duration(cfg.Events.TTL) != 2*time.Hour {
		t.Errorf("run=%+v events=%+v", cfg.Run, cfg.Events)
	}
	api, off := cfg.Rules[0], cfg.Rules[1]
	if api.Timeout != 1500*time.Millisecond || api.FailAfter != 1 || api.PassAfter != 2 || len(api.Tags) != 2 {
		t.Errorf("api = %+v", api)
	}
	if api.Params["url"] != "https://example.com" || api.Check["status"] != 200 {
		t.Errorf("api params/check = %v %v", api.Params, api.Check)
	}
	if off.Enabled || off.Timeout != 5*time.Second || off.FailAfter != 3 {
		t.Errorf("off = %+v", off)
	}
	if api.Source != cfg.Path {
		t.Errorf("source = %q", api.Source)
	}
}

func TestInclude(t *testing.T) {
	dir := t.TempDir()
	main := write(t, dir, "config.yaml", "name: main\ninclude: [conf.d/*.yaml, missing/*.yaml]\nrules:\n  - {name: first, module: file}\n")
	write(t, dir, "conf.d/20-b.yaml", "rules:\n  - {name: b, module: file}\n")
	write(t, dir, "conf.d/10-a.yaml", "rules:\n  - {name: a1, module: file}\n  - {name: a2, module: http}\n")
	write(t, dir, "conf.d/ignored.txt", "not yaml")
	cfg, err := Load(main)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range cfg.Rules {
		names = append(names, r.Name)
	}
	if strings.Join(names, ",") != "first,a1,a2,b" {
		t.Errorf("rules = %v", names)
	}
	if cfg.Rules[3].Source != filepath.Join(dir, "conf.d/20-b.yaml") {
		t.Errorf("source = %q", cfg.Rules[3].Source)
	}
}

func TestIncludeOnlyRulesKey(t *testing.T) {
	dir := t.TempDir()
	main := write(t, dir, "config.yaml", "name: main\ninclude: [conf.d/*.yaml]\n")
	write(t, dir, "conf.d/a.yaml", "name: other\nrules: []\n")
	if _, err := Load(main); err == nil || !strings.Contains(err.Error(), "a.yaml") {
		t.Errorf("include with extra keys must fail naming the file, got %v", err)
	}
}

func TestDuplicateRule(t *testing.T) {
	dir := t.TempDir()
	main := write(t, dir, "config.yaml", "name: main\ninclude: [conf.d/*.yaml]\nrules:\n  - {name: dup, module: file}\n")
	write(t, dir, "conf.d/a.yaml", "rules:\n  - {name: dup, module: file}\n")
	_, err := Load(main)
	if err == nil || !strings.Contains(err.Error(), "config.yaml") || !strings.Contains(err.Error(), "a.yaml") {
		t.Errorf("duplicate rule error must name both files, got %v", err)
	}
}

func TestWholeConfigErrors(t *testing.T) {
	cases := map[string]string{
		"unknown key":      "name: main\nrulez: []\n",
		"missing name":     "rules: []\n",
		"bad name":         "name: Bad/Name\n",
		"invalid yaml":     "name: [unclosed\n",
		"bad size":         "name: main\nrun: {max_field_size: 10XB}\n",
		"bad level":        "name: main\nlog: {level: loud}\n",
		"bad format":       "name: main\nlog: {format: xml}\n",
		"zero parallel":    "name: main\nrun: {parallel: 0}\n",
		"bad defaults":     "name: main\ndefaults: {fail_after: 0}\n",
		"rule has no name": "name: main\nrules:\n  - {module: file}\n",
	}
	for name, content := range cases {
		if _, err := Load(write(t, t.TempDir(), "c.yaml", content)); err == nil {
			t.Errorf("%s: must be a config error", name)
		}
	}
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("missing file must be an error")
	}
}

func TestRuleLevelErrors(t *testing.T) {
	content := `
name: main
rules:
  - {name: ok, module: file}
  - {name: typo, module: file, parms: {path: /x}}
  - {name: nomodule}
  - {name: Bad_Name, module: file}
  - {name: zero, module: file, fail_after: 0}
  - {name: escape, module: "x/../../other/tool"}
`
	cfg, err := Load(write(t, t.TempDir(), "c.yaml", content))
	if err != nil {
		t.Fatalf("rule-level problems must not fail the load: %v", err)
	}
	if len(cfg.Rules) != 6 || cfg.Rules[0].Err != nil {
		t.Fatalf("rules = %+v", cfg.Rules)
	}
	for _, r := range cfg.Rules[1:] {
		if r.Err == nil {
			t.Errorf("rule %q must carry an error", r.Name)
		}
	}
	if !strings.Contains(cfg.Rules[1].Err.Error(), "parms") {
		t.Errorf("typo error = %v", cfg.Rules[1].Err)
	}
}

func TestEnvExpansion(t *testing.T) {
	t.Setenv("OC_TOKEN", "s3cret")
	content := `
name: main
rules:
  - name: a
    module: http
    params:
      url: "https://example.com/${OC_TOKEN}/x"
      headers: {Authorization: "Bearer ${OC_TOKEN}"}
      list: ["${OC_TOKEN}", "literal $${OC_TOKEN}", "plain $HOME and pid $$"]
    check:
      body: {contains: "${OC_TOKEN}"}
`
	cfg, err := Load(write(t, t.TempDir(), "c.yaml", content))
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Rules[0].Params
	if p["url"] != "https://example.com/s3cret/x" {
		t.Errorf("url = %v", p["url"])
	}
	if p["headers"].(map[string]any)["Authorization"] != "Bearer s3cret" {
		t.Errorf("headers = %v", p["headers"])
	}
	list := p["list"].([]any)
	if list[0] != "s3cret" || list[1] != "literal ${OC_TOKEN}" || list[2] != "plain $HOME and pid $$" {
		t.Errorf("list = %v", list)
	}
	body := cfg.Rules[0].Check["body"].(map[string]any)
	if body["contains"] != "${OC_TOKEN}" {
		t.Errorf("check must not be expanded, got %v", body["contains"])
	}
}

func TestEnvUndefinedIsRuleError(t *testing.T) {
	content := "name: main\nrules:\n  - {name: a, module: http, params: {url: \"${OC_SURELY_UNDEFINED}\"}}\n  - {name: b, module: file}\n"
	cfg, err := Load(write(t, t.TempDir(), "c.yaml", content))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rules[0].Err == nil || !strings.Contains(cfg.Rules[0].Err.Error(), "OC_SURELY_UNDEFINED") {
		t.Errorf("err = %v", cfg.Rules[0].Err)
	}
	if cfg.Rules[1].Err != nil {
		t.Errorf("other rules must be unaffected: %v", cfg.Rules[1].Err)
	}
}

func TestYAMLAnchorsAcrossRules(t *testing.T) {
	content := `
name: main
rules:
  - name: a
    module: http
    params: &common {url: "https://example.com", headers: {X-A: b}}
    check: &ok {status: 200}
  - name: b
    module: http
    params: *common
    check: *ok
  - name: c
    module: http
    params:
      <<: *common
      method: POST
`
	cfg, err := Load(write(t, t.TempDir(), "c.yaml", content))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range cfg.Rules {
		if r.Err != nil {
			t.Errorf("rule %s: %v", r.Name, r.Err)
		}
	}
	if cfg.Rules[1].Params["url"] != "https://example.com" || cfg.Rules[1].Check["status"] != 200 {
		t.Errorf("rule b = %+v", cfg.Rules[1])
	}
	if cfg.Rules[2].Params["url"] != "https://example.com" || cfg.Rules[2].Params["method"] != "POST" {
		t.Errorf("rule c = %+v", cfg.Rules[2])
	}
}

func TestOutputTail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	load := func(head, rule string) *Config {
		t.Helper()
		cfg, err := Load(write(t, t.TempDir(), "config.yaml", "name: main\n"+head+"rules:\n  - name: a\n    module: command\n"+rule))
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	if got := load("", "").Rules[0].OutputTail; got != 10<<10 {
		t.Errorf("default output_tail = %d, want 10240", got)
	}
	if got := load("defaults: {output_tail: 1k}\n", "").Rules[0].OutputTail; got != 1<<10 {
		t.Errorf("defaults.output_tail 1k gives %d", got)
	}
	if got := load("defaults: {output_tail: 1k}\n", "    output_tail: 20k\n").Rules[0].OutputTail; got != 20<<10 {
		t.Errorf("rule output_tail 20k gives %d", got)
	}
	r := load("", "    output_tail: 0\n").Rules[0]
	if r.Err != nil || r.OutputTail != 0 {
		t.Errorf("output_tail 0 must switch saving off: %d, %v", r.OutputTail, r.Err)
	}
}

func TestLoadRemembersIncludeFiles(t *testing.T) {
	dir := t.TempDir()
	main := write(t, dir, "config.yaml", "name: main\ninclude: [conf.d/*.yaml, missing/*.yaml]\n")
	write(t, dir, "conf.d/20-b.yaml", "rules:\n  - {name: b, module: file}\n")
	write(t, dir, "conf.d/10-a.yaml", "rules:\n  - {name: a, module: file}\n")
	cfg, err := Load(main)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "conf.d/10-a.yaml"), filepath.Join(dir, "conf.d/20-b.yaml")}
	if strings.Join(cfg.Includes, ",") != strings.Join(want, ",") {
		t.Errorf("Includes = %v, want %v", cfg.Includes, want)
	}
}

func TestRawParamsKeepVariables(t *testing.T) {
	t.Setenv("TOKEN", "s3cret")
	path := write(t, t.TempDir(), "config.yaml", `name: main
rules:
  - name: a
    module: http
    params:
      url: "https://x/?t=${TOKEN}"
      n: 3
      list: [a, "${TOKEN}"]
      nested: {h: "Bearer ${TOKEN}"}
      esc: "$${TOKEN}"
  - name: bare
    module: file
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	r := cfg.Rules[0]
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if r.Params["url"] != "https://x/?t=s3cret" || r.Params["esc"] != "${TOKEN}" {
		t.Errorf("params = %v", r.Params)
	}
	if r.Params["nested"].(map[string]any)["h"] != "Bearer s3cret" {
		t.Errorf("params = %v", r.Params)
	}
	raw := r.RawParams
	if raw["url"] != "https://x/?t=${TOKEN}" || raw["n"] != 3 || raw["esc"] != "$${TOKEN}" {
		t.Errorf("raw params = %v", raw)
	}
	if raw["list"].([]any)[1] != "${TOKEN}" || raw["nested"].(map[string]any)["h"] != "Bearer ${TOKEN}" {
		t.Errorf("raw params = %v", raw)
	}
	if cfg.Rules[1].RawParams != nil {
		t.Errorf("a rule without params has raw params %v", cfg.Rules[1].RawParams)
	}
}

func TestRawParamsOfARuleWithAnUndefinedVariable(t *testing.T) {
	path := write(t, t.TempDir(), "config.yaml",
		"name: main\nrules:\n  - name: a\n    module: http\n    params: {url: \"https://x/${ONECHECK_TEST_UNSET}\"}\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	r := cfg.Rules[0]
	if r.Err == nil || r.Params != nil {
		t.Fatalf("rule = %+v", r)
	}
	if r.RawParams["url"] != "https://x/${ONECHECK_TEST_UNSET}" {
		t.Errorf("raw params = %v", r.RawParams)
	}
}

func TestEventParamsDefaultsAndOverride(t *testing.T) {
	load := func(defaults string) []Rule {
		t.Helper()
		path := write(t, t.TempDir(), "config.yaml", "name: main\n"+defaults+`rules:
  - name: plain
    module: file
  - name: shown
    module: file
    event_params: true
  - name: hidden
    module: file
    event_params: false
`)
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return cfg.Rules
	}
	got := func(rules []Rule) [3]bool {
		return [3]bool{rules[0].EventParams, rules[1].EventParams, rules[2].EventParams}
	}
	if g := got(load("")); g != [3]bool{true, true, false} {
		t.Errorf("default: %v", g)
	}
	if g := got(load("defaults: {event_params: false}\n")); g != [3]bool{false, true, false} {
		t.Errorf("defaults.event_params false: %v", g)
	}
}

func TestEventParamsNotBoolean(t *testing.T) {
	path := write(t, t.TempDir(), "config.yaml",
		"name: main\nrules:\n  - name: a\n    module: file\n    event_params: maybe\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rules[0].Err == nil {
		t.Error("event_params: maybe must make the rule invalid")
	}
}

func TestRemindEveryDefaultsAndOverride(t *testing.T) {
	load := func(defaults string) []Rule {
		t.Helper()
		path := write(t, t.TempDir(), "config.yaml", "name: main\n"+defaults+`rules:
  - name: plain
    module: file
  - name: own
    module: file
    remind_every: 10m
  - name: off
    module: file
    remind_every: 0
`)
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range cfg.Rules {
			if r.Err != nil {
				t.Fatalf("rule %s: %v", r.Name, r.Err)
			}
		}
		return cfg.Rules
	}
	type remind struct {
		every    time.Duration
		explicit bool
	}
	got := func(rules []Rule) [3]remind {
		var out [3]remind
		for i, r := range rules {
			out[i] = remind{r.RemindEvery, r.RemindExplicit}
		}
		return out
	}
	own, off := remind{10 * time.Minute, true}, remind{0, true}
	if g := got(load("")); g != [3]remind{{time.Hour, false}, own, off} {
		t.Errorf("built-in default: %v", g)
	}
	if g := got(load("defaults: {remind_every: 2h}\n")); g != [3]remind{{2 * time.Hour, true}, own, off} {
		t.Errorf("defaults.remind_every 2h: %v", g)
	}
	if g := got(load("defaults: {remind_every: 0}\n")); g != [3]remind{{0, true}, own, off} {
		t.Errorf("defaults.remind_every 0: %v", g)
	}
}

func TestRemindEveryInvalid(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(write(t, dir, "config.yaml", `name: main
rules:
  - name: negative
    module: file
    remind_every: -5m
  - name: words
    module: file
    remind_every: soon
  - name: typo
    module: file
    remind_evry: 1h
`))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range cfg.Rules[:2] {
		if r.Err == nil || !strings.Contains(r.Err.Error(), "invalid duration") {
			t.Errorf("rule %s: err = %v, want an invalid duration", r.Name, r.Err)
		}
	}
	if err := cfg.Rules[2].Err; err == nil || !strings.Contains(err.Error(), "event_params, event_output, remind_every, params") {
		t.Errorf("typo: err = %v, want the known keys including remind_every", err)
	}
	if _, err := Load(write(t, dir, "bad.yaml", "name: main\ndefaults: {remind_every: -1h}\n")); err == nil {
		t.Error("a negative defaults.remind_every must make the config unusable")
	}
}

func TestEventOutput(t *testing.T) {
	cfg, err := Load(write(t, t.TempDir(), "config.yaml", `name: main
rules:
  - name: plain
    module: command
  - name: shown
    module: command
    event_output: [stderr, stdout]
  - name: word
    module: command
    event_output: stderr
  - name: typo
    module: command
    event_outputs: [stderr]
`))
	if err != nil {
		t.Fatal(err)
	}
	r := cfg.Rules
	if r[0].Err != nil || len(r[0].EventOutput) != 0 {
		t.Errorf("plain: %v %v", r[0].EventOutput, r[0].Err)
	}
	if r[1].Err != nil || strings.Join(r[1].EventOutput, ",") != "stderr,stdout" {
		t.Errorf("shown: %v %v", r[1].EventOutput, r[1].Err)
	}
	if r[2].Err == nil {
		t.Error("event_output must be a list")
	}
	if r[3].Err == nil || !strings.Contains(r[3].Err.Error(), "event_params, event_output, remind_every") {
		t.Errorf("typo: %v", r[3].Err)
	}
}

func TestRuleWithAnUnknownKeyKeepsItsOtherKeys(t *testing.T) {
	cfg, err := Load(write(t, t.TempDir(), "config.yaml", `name: main
rules:
  - name: typo
    module: command
    tags: [db]
    event_params: false
    remind_every: 0
    fail_after: 3
    chek: {exit_code: 0}
`))
	if err != nil {
		t.Fatal(err)
	}
	r := cfg.Rules[0]
	if r.Err == nil || !strings.Contains(r.Err.Error(), "chek") {
		t.Fatalf("err = %v, want the unknown key", r.Err)
	}
	if r.EventParams || r.RemindEvery != 0 || !r.RemindExplicit || r.FailAfter != 3 || r.Module != "command" || len(r.Tags) != 1 {
		t.Errorf("a broken rule must still hide its details and keep its settings: %+v", r)
	}
}

func TestEventOutputDropsDuplicates(t *testing.T) {
	cfg, err := Load(write(t, t.TempDir(), "config.yaml", `name: main
rules:
  - name: c
    module: command
    event_output: [stderr, stdout, stderr]
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.Rules[0].EventOutput, ","); got != "stderr,stdout" {
		t.Errorf("event_output = %s", got)
	}
}

// isolateDefaults points the default config directories at empty temp dirs.
func isolateDefaults(t *testing.T) (system, home string) {
	t.Helper()
	system, home = t.TempDir(), t.TempDir()
	t.Cleanup(SetSystemDir(system))
	t.Setenv("HOME", home)
	t.Setenv("ONECHECK_CONFIG", "")
	return system, home
}

func TestDiscoverFile(t *testing.T) {
	isolateDefaults(t)
	f := write(t, t.TempDir(), "x.yaml", "name: x\n")
	got, err := Discover(f)
	if err != nil || got.Dir || got.Source != SourceFlag || got.Path != f || strings.Join(got.Files, ",") != f {
		t.Errorf("Discover(file) = %+v, %v", got, err)
	}
	if _, err := Discover("/nonexistent/x.yaml"); err == nil {
		t.Error("explicit missing path must be an error")
	}
}

func TestDiscoverDir(t *testing.T) {
	isolateDefaults(t)
	dir := t.TempDir()
	b := write(t, dir, "b.yml", "name: b\n")
	a := write(t, dir, "a.yaml", "name: a\n")
	write(t, dir, ".hidden.yaml", "name: h\n")
	write(t, dir, "notes.txt", "x")
	write(t, dir, "sub/c.yaml", "name: c\n")
	if err := os.Mkdir(filepath.Join(dir, "d.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(dir)
	if err != nil || !got.Dir || got.Path != dir || strings.Join(got.Files, ",") != a+","+b {
		t.Errorf("Discover(dir) = %+v, %v", got, err)
	}
}

func TestDiscoverEmptyDir(t *testing.T) {
	isolateDefaults(t)
	dir := t.TempDir()
	write(t, dir, "conf.d/a.yaml", "rules: []\n")
	if _, err := Discover(dir); err == nil || !strings.Contains(err.Error(), "no config files") {
		t.Errorf("err = %v", err)
	}
}

func TestDiscoverDefaultOrder(t *testing.T) {
	system, home := isolateDefaults(t)
	homeDir := filepath.Join(home, ".config", "onecheck")
	homeCfg := write(t, homeDir, "config.yaml", "name: home\n")
	got, err := Discover("")
	if err != nil || got.Path != homeDir || !got.Dir || got.Source != SourceDefault || got.Files[0] != homeCfg {
		t.Errorf("empty system dir must fall through to home: %+v, %v", got, err)
	}
	sysCfg := write(t, system, "config.yaml", "name: sys\n")
	if got, _ := Discover(""); got.Path != system || got.Files[0] != sysCfg {
		t.Errorf("system dir must win: %+v", got)
	}
	envDir := t.TempDir()
	write(t, envDir, "e.yaml", "name: e\n")
	t.Setenv("ONECHECK_CONFIG", envDir)
	if got, _ := Discover(""); got.Path != envDir || got.Source != SourceEnv || !got.Dir {
		t.Errorf("env must beat defaults: %+v", got)
	}
	flag := write(t, t.TempDir(), "f.yaml", "name: f\n")
	if got, _ := Discover(flag); got.Path != flag || got.Source != SourceFlag {
		t.Errorf("flag must beat env: %+v", got)
	}
}

func TestDiscoverNotFound(t *testing.T) {
	system, home := isolateDefaults(t)
	_, err := Discover("")
	if err == nil || !strings.Contains(err.Error(), system) || !strings.Contains(err.Error(), filepath.Join(home, ".config", "onecheck")) || !strings.Contains(err.Error(), "--config") {
		t.Errorf("error must list the directories tried, got %v", err)
	}
}
