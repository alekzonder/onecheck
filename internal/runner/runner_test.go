package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/onecheck/internal/config"
	"github.com/alekzonder/onecheck/internal/result"
	"github.com/alekzonder/onecheck/internal/store"
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

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// env is a config living in its own temp dir, with a data dir for files the
// rules inspect.
type env struct {
	t    *testing.T
	dir  string
	data string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{t: t, dir: dir, data: filepath.Join(dir, "data")}
	if err := os.MkdirAll(e.data, 0o755); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) load(rules string) *config.Config {
	e.t.Helper()
	content := fmt.Sprintf("name: main\npaths:\n  modules: %s\n  results: results\n  state: state\n  events: events\nlog: {dir: log}\nrules:\n%s",
		modDir, strings.ReplaceAll(rules, "DATA", e.data))
	path := filepath.Join(e.dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		e.t.Fatal(err)
	}
	return cfg
}

func (e *env) run(rules string, opts Options) []result.Result {
	e.t.Helper()
	res, err := Run(context.Background(), e.load(rules), opts, quiet)
	if err != nil {
		e.t.Fatalf("Run: %v", err)
	}
	return res
}

func (e *env) touch(name, content string) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(e.data, name), []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) events() []result.Event {
	e.t.Helper()
	files, _ := filepath.Glob(filepath.Join(e.dir, "events", "*.json"))
	var out []result.Event
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			e.t.Fatal(err)
		}
		var ev result.Event
		if err := json.Unmarshal(data, &ev); err != nil {
			e.t.Fatalf("%s: %v", f, err)
		}
		out = append(out, ev)
	}
	return out
}

func (e *env) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(e.dir, rel))
	return err == nil
}

const twoRules = `
  - name: present
    module: file
    params: {path: DATA/a.txt}
    check: {exists: true, content: {contains: hello}}
  - name: absent
    module: file
    tags: [extra]
    params: {path: DATA/missing.txt}
    check: {exists: true}
`

func TestRunPassAndFail(t *testing.T) {
	e := newEnv(t)
	e.touch("a.txt", "hello world")
	res := e.run(twoRules, Options{})
	if len(res) != 2 || res[0].Rule != "present" || res[1].Rule != "absent" {
		t.Fatalf("results must come in config order: %+v", res)
	}
	p, a := res[0], res[1]
	if p.Status != result.Pass || p.State != "pass" || p.Transition != nil || p.Config != "main" || p.Module != "file" || p.ModuleVersion == "" {
		t.Errorf("present = %+v", p)
	}
	if a.Status != result.Fail || len(a.Failures) != 1 || a.Failures[0].Path != "exists" || a.State != "fail" {
		t.Errorf("absent = %+v", a)
	}
	if a.Transition == nil || *a.Transition != "fail" || a.Consecutive != 1 || len(a.Tags) != 1 {
		t.Errorf("absent = %+v", a)
	}
	for _, f := range []string{"results/main/present.json", "results/main/absent.json", "state/main/present.json", "state/main/absent.json"} {
		if !e.exists(f) {
			t.Errorf("%s was not written", f)
		}
	}
}

const watchedRule = `
  - name: watched
    module: file
    fail_after: 2
    pass_after: 2
    params: {path: DATA/w.txt}
    check: {exists: true}
`

func TestRunTransitionWritesEvent(t *testing.T) {
	e := newEnv(t)
	rules := watchedRule
	e.touch("w.txt", "x")
	if res := e.run(rules, Options{}); res[0].Status != result.Pass || len(e.events()) != 0 {
		t.Fatalf("a new passing rule must not emit an event: %+v", res)
	}
	os.Remove(filepath.Join(e.data, "w.txt"))
	res := e.run(rules, Options{})
	if res[0].Status != result.Fail || res[0].State != "pass" || res[0].Transition != nil || res[0].Consecutive != 1 || len(e.events()) != 0 {
		t.Fatalf("first failure of two must not change state: %+v", res[0])
	}
	res = e.run(rules, Options{})
	evs := e.events()
	if res[0].State != "fail" || res[0].Transition == nil || len(evs) != 1 {
		t.Fatalf("second failure must flip the state and emit one event: %+v, events=%d", res[0], len(evs))
	}
	ev := evs[0]
	if ev.Transition != "fail" || ev.From != "pass" || ev.To != "fail" || ev.Rule != "watched" || ev.Config != "main" {
		t.Errorf("event = %+v", ev)
	}
	if ev.Result.Status != result.Fail || len(ev.Result.Failures) != 1 || ev.ExpiresAt.Sub(ev.At).Hours() != 24 {
		t.Errorf("event = %+v", ev)
	}
}

func TestRunRecoveryWritesEvent(t *testing.T) {
	e := newEnv(t)
	rules := watchedRule
	e.run(rules, Options{})
	e.run(rules, Options{})
	e.touch("w.txt", "x")
	res := e.run(rules, Options{})
	if res[0].Status != result.Pass || res[0].State != "fail" || len(e.events()) != 1 {
		t.Fatalf("first pass of two must not recover yet: %+v", res[0])
	}
	res = e.run(rules, Options{})
	if res[0].State != "pass" || res[0].Transition == nil || *res[0].Transition != "recovered" {
		t.Fatalf("second pass must recover: %+v", res[0])
	}
	var recovered int
	for _, ev := range e.events() {
		if ev.Transition == "recovered" && ev.From == "fail" && ev.To == "pass" {
			recovered++
		}
	}
	if recovered != 1 {
		t.Errorf("want one recovered event, events = %+v", e.events())
	}
}

func TestRunMissingModule(t *testing.T) {
	e := newEnv(t)
	e.touch("a.txt", "hello")
	rules := twoRules + `
  - name: ghost
    module: nosuchmodule
    params: {x: 1}
`
	res := e.run(rules, Options{})
	if len(res) != 3 || res[0].Status != result.Pass {
		t.Fatalf("other rules must still run: %+v", res)
	}
	g := res[2]
	if g.Status != result.Error || g.Error == nil || !strings.Contains(*g.Error, "not installed") || g.State != "fail" {
		t.Errorf("ghost = %+v", g)
	}
}

func TestRunMissingModulesDir(t *testing.T) {
	e := newEnv(t)
	cfg := e.load(twoRules)
	cfg.Paths.Modules = filepath.Join(e.dir, "no-such-dir")
	res, err := Run(context.Background(), cfg, Options{}, quiet)
	if err != nil {
		t.Fatalf("a missing modules dir must not abort the run: %v", err)
	}
	for _, r := range res {
		if r.Status != result.Error {
			t.Errorf("%s: status = %s, want error", r.Rule, r.Status)
		}
	}
}

func TestRunBrokenRuleOthersContinue(t *testing.T) {
	e := newEnv(t)
	e.touch("a.txt", "hello")
	rules := `
  - name: bad-operator
    module: file
    params: {path: DATA/a.txt}
    check: {exists: {equalz: true}}
  - name: bad-field
    module: file
    params: {path: DATA/a.txt}
    check: {nonexistent_field: 1}
  - name: bad-params
    module: file
    params: {pth: DATA/a.txt}
  - name: good
    module: file
    params: {path: DATA/a.txt}
    check: {exists: true}
  - name: no-check
    module: file
    params: {path: DATA/a.txt}
`
	res := e.run(rules, Options{})
	for i, want := range []string{"equalz", "nonexistent_field", "params"} {
		if res[i].Status != result.Error || res[i].Error == nil || !strings.Contains(*res[i].Error, want) {
			t.Errorf("%s: %+v (error should mention %q)", res[i].Rule, res[i], want)
		}
	}
	if res[3].Status != result.Pass || res[4].Status != result.Pass {
		t.Errorf("good rules must pass: %+v %+v", res[3], res[4])
	}
}

func TestRunModuleErrorAndTimeout(t *testing.T) {
	e := newEnv(t)
	rules := `
  - name: cannot-start
    module: command
    params: {argv: [/nonexistent/program]}
    check: {exit_code: 0}
  - name: slow
    module: command
    timeout: 300ms
    params: {run: "sleep 30"}
    check: {exit_code: 0}
  - name: exit-code
    module: command
    params: {run: "echo '{\"ok\": true}'; exit 2"}
    check:
      exit_code: 2
      stdout: {json: {assert: [{path: ok, equals: true}]}}
`
	res := e.run(rules, Options{})
	if res[0].Status != result.Error || res[1].Status != result.Error {
		t.Errorf("module failures must be errors: %+v %+v", res[0], res[1])
	}
	if res[1].Error == nil || !strings.Contains(*res[1].Error, "timed out") || res[1].DurationMS > 5000 {
		t.Errorf("slow = %+v", res[1])
	}
	if res[2].Status != result.Pass {
		t.Errorf("exit-code = %+v", res[2])
	}
}

func TestRunTruncatedFieldIsError(t *testing.T) {
	e := newEnv(t)
	e.touch("big.txt", strings.Repeat("a", 5000))
	cfg := e.load(`
  - name: big
    module: file
    params: {path: DATA/big.txt}
    check: {content: {contains: zzz}}
  - name: big-size-only
    module: file
    params: {path: DATA/big.txt}
    check: {size: {gt: 1KB}}
`)
	cfg.Run.MaxFieldSize = 100
	res, err := Run(context.Background(), cfg, Options{}, quiet)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Status != result.Error || res[0].Error == nil || !strings.Contains(*res[0].Error, "truncated") {
		t.Errorf("big = %+v", res[0])
	}
	if res[1].Status != result.Pass {
		t.Errorf("a rule not using the big field must be unaffected: %+v", res[1])
	}
}

func TestRunEventWriteFailureKeepsState(t *testing.T) {
	e := newEnv(t)
	cfg := e.load(twoRules)
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	if _, err := store.Open(cfg); err != nil {
		t.Fatal(err)
	}
	// A read-only events directory: no event can be written.
	if err := os.Chmod(cfg.EventsDir(), 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(cfg.EventsDir(), 0o755) }()
	_, err := Run(context.Background(), cfg, Options{}, quiet)
	if err == nil {
		t.Fatal("a transition whose event cannot be written must fail the run")
	}
	if e.exists("state/main/absent.json") {
		t.Error("the state must not advance when its event was not written")
	}
}

func TestRunLocked(t *testing.T) {
	e := newEnv(t)
	cfg := e.load(twoRules)
	s, err := store.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := s.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := Run(context.Background(), cfg, Options{}, quiet); !errors.Is(err, store.ErrLocked) {
		t.Errorf("err = %v, want ErrLocked", err)
	}
}

func TestRunFilterByRuleAndTag(t *testing.T) {
	e := newEnv(t)
	e.touch("a.txt", "hello")
	if res := e.run(twoRules, Options{Rules: []string{"present"}}); len(res) != 1 || res[0].Rule != "present" {
		t.Errorf("--rule: %+v", res)
	}
	if res := e.run(twoRules, Options{Tags: []string{"extra"}}); len(res) != 1 || res[0].Rule != "absent" {
		t.Errorf("--tag: %+v", res)
	}
	if _, err := Run(context.Background(), e.load(twoRules), Options{Rules: []string{"nope"}}, quiet); err == nil {
		t.Error("an unknown --rule must be an error")
	}
	disabled := twoRules + "  - {name: off, module: file, enabled: false, params: {path: /}}\n"
	if res := e.run(disabled, Options{}); len(res) != 2 {
		t.Errorf("disabled rules must not run: %+v", res)
	}
}

func TestRunPrunesRemovedRules(t *testing.T) {
	e := newEnv(t)
	e.touch("a.txt", "hello")
	e.run(twoRules, Options{})
	onlyFirst := strings.Split(twoRules, "  - name: absent")[0]

	e.run(onlyFirst, Options{Rules: []string{"present"}})
	if !e.exists("results/main/absent.json") {
		t.Error("a filtered run must not prune")
	}
	e.run(onlyFirst, Options{})
	if e.exists("results/main/absent.json") || e.exists("state/main/absent.json") {
		t.Error("files of a removed rule must be pruned by a full run")
	}
	if !e.exists("results/main/present.json") {
		t.Error("files of remaining rules must stay")
	}
}

func TestRunCorruptStateIsNewRule(t *testing.T) {
	e := newEnv(t)
	e.touch("a.txt", "hello")
	e.run(twoRules, Options{})
	if err := os.WriteFile(filepath.Join(e.dir, "state/main/present.json"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := e.run(twoRules, Options{})
	if res[0].Status != result.Pass || res[0].State != "pass" || res[0].Consecutive != 1 {
		t.Errorf("present = %+v", res[0])
	}
}

func TestValidateReportsAll(t *testing.T) {
	e := newEnv(t)
	cfg := e.load(`
  - {name: ok, module: file, params: {path: /tmp}, check: {exists: true}}
  - {name: bad-op, module: file, params: {path: /tmp}, check: {exists: {nope: 1}}}
  - {name: no-module, module: ghost}
  - {name: disabled-bad, module: file, enabled: false, params: {}}
`)
	errs := Validate(context.Background(), cfg)
	if len(errs) != 3 {
		t.Fatalf("want 3 errors, got %v", errs)
	}
	for i, want := range []string{"rule bad-op", "rule no-module", "rule disabled-bad"} {
		if !strings.Contains(errs[i].Error(), want) || !strings.Contains(errs[i].Error(), "config.yaml") {
			t.Errorf("error %d = %v, want it to mention %q and the source file", i, errs[i], want)
		}
	}
}
