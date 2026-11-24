package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alekzonder/onecheck/internal/config"
	"github.com/alekzonder/onecheck/internal/result"
	"github.com/alekzonder/onecheck/internal/store"
	"github.com/alekzonder/onecheck/internal/testmods"
)

var modDir string

func TestMain(m *testing.M) {
	dir, err := testmods.Build("file", "command", "http")
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

func TestRunInterruptedRecordsNothing(t *testing.T) {
	e := newEnv(t)
	e.touch("a.txt", "hello")
	pidFile := filepath.Join(e.dir, "pid")
	cfg := e.load(`
  - name: slow
    module: command
    params: {run: "sleep 60 & echo $! > ` + pidFile + `; wait"}
    check: {exit_code: 0}
  - name: healthy
    module: file
    params: {path: DATA/a.txt}
    check: {exists: true}
`)
	cfg.Run.Parallel = 1
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for i := 0; i < 200; i++ {
			if _, err := os.Stat(pidFile); err == nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := Run(ctx, cfg, Options{}, quiet)
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("an interrupted run must be an error, got %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("Run blocked for %v after cancellation", time.Since(start))
	}
	if len(e.events()) != 0 || e.exists("state/main/slow.json") || e.exists("state/main/healthy.json") || e.exists("results/main/slow.json") {
		t.Error("an interrupted run must not record state, results or events")
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("process %d started by the command survived the interruption", pid)
}

func processAlive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return true
	}
	fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
	return len(fields) == 0 || fields[0] != "Z"
}

func TestRunFilesAreNotWorldReadable(t *testing.T) {
	e := newEnv(t)
	e.run(twoRules, Options{})
	for _, pattern := range []string{"results/main/*.json", "state/main/*.json", "events/*.json"} {
		files, _ := filepath.Glob(filepath.Join(e.dir, pattern))
		if len(files) == 0 {
			t.Fatalf("no files for %s", pattern)
		}
		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0o007 != 0 {
				t.Errorf("%s has mode %04o: results may contain checked content and must not be world-readable", f, info.Mode().Perm())
			}
		}
	}
	for _, dir := range []string{"results/main", "state/main", "events"} {
		info, err := os.Stat(filepath.Join(e.dir, dir))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o007 != 0 {
			t.Errorf("directory %s has mode %04o, want no access for others", dir, info.Mode().Perm())
		}
	}
}

func TestDebugLogShowsWhatWasAskedAndCollected(t *testing.T) {
	e := newEnv(t)
	if err := os.WriteFile(filepath.Join(e.data, "f.txt"), []byte("hello "+strings.Repeat("x", 5000)), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := e.load(`
  - name: f
    module: file
    params: { path: DATA/f.txt }
    check: { exists: true, content: { contains: hello } }
`)
	if _, err := Run(context.Background(), cfg, Options{}, log); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`msg="module loaded"`, "onecheck-file",
		`msg="collecting fields"`, "f.txt", "timeout=30s",
		`msg="fields collected"`, "exists=true", "hello xxx",
		`msg="state updated"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("debug log lacks %q:\n%s", want, out)
		}
	}
	if len(out) > 4000 {
		t.Errorf("a 5000-byte field was logged in full (%d bytes of log)", len(out))
	}
}

func (e *env) saved(rule string) result.Result {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(e.dir, "results", "main", rule+".json"))
	if err != nil {
		e.t.Fatal(err)
	}
	var res result.Result
	if err := json.Unmarshal(data, &res); err != nil {
		e.t.Fatal(err)
	}
	return res
}

func TestCommandOutputIsSavedEvenWhenNotChecked(t *testing.T) {
	e := newEnv(t)
	res := e.run(`
  - name: c
    module: command
    params: { run: "echo out; echo err >&2; exit 3" }
    check: { exit_code: 0 }
`, Options{})
	if res[0].Status != result.Fail {
		t.Fatalf("status = %s", res[0].Status)
	}
	for _, got := range []map[string]any{res[0].Output, e.saved("c").Output} {
		if got["stdout"] != "out\n" || got["stderr"] != "err\n" {
			t.Errorf("output = %v", got)
		}
	}
	if ev := e.events(); len(ev) != 1 || ev[0].Result.Output["stderr"] != "err\n" {
		t.Errorf("the fail event does not carry the output: %+v", ev)
	}
}

func TestOutputIsCutToItsTail(t *testing.T) {
	e := newEnv(t)
	res := e.run(`
  - name: c
    module: command
    output_tail: 1k
    params: { run: "head -c 5000 /dev/zero | tr '\\0' a; echo THE-END" }
    check: { exit_code: 0 }
`, Options{})
	out, _ := res[0].Output["stdout"].(string)
	if !strings.HasPrefix(out, "…aaa") || !strings.HasSuffix(out, "aTHE-END\n") {
		t.Errorf("not a marked tail: %.20q ... %.20q", out, out[max(0, len(out)-20):])
	}
	if got := len(strings.TrimPrefix(out, "…")); got != 1024 {
		t.Errorf("tail is %d bytes, want 1024", got)
	}
}

func TestOutputTailZeroSavesNothing(t *testing.T) {
	e := newEnv(t)
	res := e.run(`
  - name: c
    module: command
    output_tail: 0
    params: { run: "echo out" }
    check: { exit_code: 0 }
`, Options{})
	if len(res[0].Output) != 0 {
		t.Errorf("output = %v", res[0].Output)
	}
	if !strings.Contains(string(mustRead(t, filepath.Join(e.dir, "results", "main", "c.json"))), `"output": {}`) {
		t.Error(`an empty output must be written as "output": {}`)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestTimedOutCommandKeepsItsOutput(t *testing.T) {
	e := newEnv(t)
	res := e.run(`
  - name: c
    module: command
    timeout: 300ms
    params: { run: "echo started; exec sleep 30" }
    check: { exit_code: 0 }
`, Options{})
	if res[0].Status != result.Error || !strings.Contains(*res[0].Error, "timed out") {
		t.Fatalf("status = %s error = %v", res[0].Status, res[0].Error)
	}
	if res[0].Output["stdout"] != "started\n" {
		t.Errorf("output = %v", res[0].Output)
	}
}

func TestFileModuleSavesNoOutput(t *testing.T) {
	e := newEnv(t)
	e.touch("f.txt", "secret content")
	res := e.run(`
  - name: f
    module: file
    params: { path: DATA/f.txt }
    check: { content: { contains: secret } }
`, Options{})
	if res[0].Status != result.Pass || len(res[0].Output) != 0 {
		t.Errorf("status = %s output = %v", res[0].Status, res[0].Output)
	}
}

func TestHTTPBodyAndHeadersAreSaved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Reason", "db down")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"no database"}`))
	}))
	defer srv.Close()
	e := newEnv(t)
	res := e.run(`
  - name: h
    module: http
    params: { url: `+srv.URL+` }
    check: { status: 200 }
`, Options{})
	if res[0].Status != result.Fail {
		t.Fatalf("status = %s error = %v", res[0].Status, res[0].Error)
	}
	if res[0].Output["body"] != `{"error":"no database"}` {
		t.Errorf("body = %v", res[0].Output["body"])
	}
	headers, _ := res[0].Output["headers"].(map[string]any)
	if headers["x-reason"] != "db down" {
		t.Errorf("headers = %v", res[0].Output["headers"])
	}
}

// eventJSON returns the only event of the env as raw JSON keys.
func (e *env) eventJSON() map[string]json.RawMessage {
	e.t.Helper()
	files, _ := filepath.Glob(filepath.Join(e.dir, "events", "*.json"))
	if len(files) != 1 {
		e.t.Fatalf("want one event, got %d", len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		e.t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		e.t.Fatal(err)
	}
	return out
}

func TestEventCarriesRawParams(t *testing.T) {
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Token")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	t.Setenv("ONECHECK_TEST_TOKEN", "s3cret")
	e := newEnv(t)
	e.run(`
  - name: api
    module: http
    params:
      url: `+srv.URL+`
      headers: {X-Token: "${ONECHECK_TEST_TOKEN}"}
    check: {status: 200}
`, Options{})
	if gotToken != "s3cret" {
		t.Errorf("the module must get the expanded value, the server saw %q", gotToken)
	}
	ev := e.eventJSON()
	if string(ev["redacted"]) != "false" {
		t.Errorf("redacted = %s", ev["redacted"])
	}
	params := string(ev["params"])
	if !strings.Contains(params, "${ONECHECK_TEST_TOKEN}") || strings.Contains(params, "s3cret") {
		t.Errorf("params = %s", params)
	}
	var whole []byte
	for _, v := range ev {
		whole = append(whole, v...)
	}
	if strings.Contains(string(whole), "s3cret") {
		t.Errorf("the event holds the expanded secret: %s", whole)
	}
}

func TestEventParamsHidden(t *testing.T) {
	e := newEnv(t)
	e.run(`
  - name: gone
    module: file
    event_params: false
    params: {path: DATA/missing.txt}
    check: {exists: true}
`, Options{})
	ev := e.eventJSON()
	if string(ev["params"]) != "null" || string(ev["redacted"]) != "true" {
		t.Errorf("params = %s, redacted = %s", ev["params"], ev["redacted"])
	}
}

func TestEventWithoutParams(t *testing.T) {
	e := newEnv(t)
	e.run(`
  - name: bare
    module: command
    check: {exit_code: 0}
`, Options{})
	ev := e.eventJSON()
	if string(ev["params"]) != "null" || string(ev["redacted"]) != "false" {
		t.Errorf("params = %s, redacted = %s", ev["params"], ev["redacted"])
	}
}

func TestEventOfAnInvalidRuleHasNoExpandedParams(t *testing.T) {
	e := newEnv(t)
	e.run(`
  - name: broken
    module: http
    params: {url: "https://example.com/${ONECHECK_TEST_UNSET_VAR}"}
    check: {status: 200}
`, Options{})
	ev := e.eventJSON()
	if !strings.Contains(string(ev["params"]), "${ONECHECK_TEST_UNSET_VAR}") {
		t.Errorf("params = %s", ev["params"])
	}
}
