package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/onecheck/internal/config"
	"github.com/alekzonder/onecheck/internal/result"
	"github.com/alekzonder/onecheck/internal/state"
)

func testConfig(t *testing.T, name string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{Name: name, Path: filepath.Join(dir, name+".yaml")}
	cfg.Paths.Results = filepath.Join(dir, "results")
	cfg.Paths.State = filepath.Join(dir, "state")
	cfg.Paths.Events = filepath.Join(dir, "events")
	return cfg
}

func open(t *testing.T, cfg *config.Config) *Store {
	t.Helper()
	s, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	for _, content := range []string{"first", "second"} {
		if err := WriteFileAtomic(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(path)
		if string(got) != content {
			t.Errorf("content = %q, want %q", got, content)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
	if err := WriteFileAtomic(filepath.Join(dir, "missing-dir", "x"), []byte("a"), 0o644); err == nil {
		t.Error("writing into a missing directory must fail")
	}
}

func TestOpenCreatesDirs(t *testing.T) {
	cfg := testConfig(t, "main")
	open(t, cfg)
	for _, dir := range []string{cfg.ResultsDir(), cfg.StateDir(), cfg.EventsDir()} {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			t.Errorf("%s was not created: %v", dir, err)
		}
	}
}

func TestLockExclusive(t *testing.T) {
	cfg := testConfig(t, "main")
	s1, s2 := open(t, cfg), open(t, cfg)
	unlock, err := s1.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Lock(); !errors.Is(err, ErrLocked) {
		t.Errorf("second lock: err = %v, want ErrLocked", err)
	}
	unlock()
	unlock2, err := s2.Lock()
	if err != nil {
		t.Errorf("lock must be free after unlock: %v", err)
	} else {
		unlock2()
	}
}

func TestLockOwnerMismatch(t *testing.T) {
	cfg := testConfig(t, "main")
	unlock, err := open(t, cfg).Lock()
	if err != nil {
		t.Fatal(err)
	}
	unlock()

	other := *cfg
	other.Path = filepath.Join(filepath.Dir(cfg.Path), "copy.yaml")
	_, err = open(t, &other).Lock()
	if err == nil || !strings.Contains(err.Error(), cfg.Path) || !strings.Contains(err.Error(), other.Path) {
		t.Errorf("a second config file with the same name must be refused naming both paths, got %v", err)
	}
	if unlock, err := open(t, cfg).Lock(); err != nil {
		t.Errorf("the owner must still be able to lock: %v", err)
	} else {
		unlock()
	}
}

func TestStateRoundTrip(t *testing.T) {
	s := open(t, testConfig(t, "main"))
	if st, err := s.LoadState("api"); st != nil || err != nil {
		t.Errorf("absent state must be nil, nil; got %v %v", st, err)
	}
	want := state.State{State: state.Fail, Since: time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC), ConsecutiveFail: 3}
	if err := s.SaveState("api", want); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadState("api")
	if err != nil || got == nil || *got != want {
		t.Errorf("got %+v %v", got, err)
	}
}

func TestLoadStateCorrupt(t *testing.T) {
	cfg := testConfig(t, "main")
	s := open(t, cfg)
	for _, content := range []string{"", "{not json", `{"state":"weird"}`} {
		if err := os.WriteFile(filepath.Join(cfg.StateDir(), "api.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		st, err := s.LoadState("api")
		if st != nil || !errors.Is(err, ErrCorrupt) {
			t.Errorf("content %q: got %v, %v; want nil, ErrCorrupt", content, st, err)
		}
	}
}

func TestWriteResult(t *testing.T) {
	cfg := testConfig(t, "main")
	s := open(t, cfg)
	r := result.Result{Config: "main", Rule: "api", Module: "http", Status: result.Pass, State: state.Pass}
	if err := s.WriteResult(r); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(cfg.ResultsDir(), "api.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"config", "rule", "module", "module_version", "tags", "status", "state", "transition",
		"consecutive", "fail_after", "pass_after", "started_at", "duration_ms", "failures", "error"} {
		if _, ok := m[key]; !ok {
			t.Errorf("result JSON lacks %q", key)
		}
	}
	if m["transition"] != nil || m["error"] != nil {
		t.Errorf("transition and error must be null: %v", m)
	}
	if f, ok := m["failures"].([]any); !ok || len(f) != 0 {
		t.Errorf("failures must be [], got %v", m["failures"])
	}
	if tags, ok := m["tags"].([]any); !ok || len(tags) != 0 {
		t.Errorf("tags must be [], got %v", m["tags"])
	}
}

func TestWriteEventName(t *testing.T) {
	cfg := testConfig(t, "main")
	s := open(t, cfg)
	at := time.Date(2026, 11, 23, 10, 2, 0, 0, time.UTC)
	id := EventID(at, "main", "api-health", state.TransitionFail)
	if id != "20261123T100200Z-main-api-health-fail" {
		t.Fatalf("id = %q", id)
	}
	ev := result.Event{ID: id, Config: "main", Rule: "api-health", Transition: state.TransitionFail,
		From: state.Pass, To: state.Fail, At: at, ExpiresAt: at.Add(24 * time.Hour)}
	if err := s.WriteEvent(ev); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(cfg.EventsDir(), id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["expires_at"] != "2026-11-24T10:02:00Z" || m["transition"] != "fail" || m["from"] != "pass" || m["to"] != "fail" {
		t.Errorf("event = %v", m)
	}
	if _, ok := m["result"].(map[string]any); !ok {
		t.Errorf("event must embed the result: %v", m)
	}
}

func TestUnsafeRuleNameStaysInsideDir(t *testing.T) {
	cfg := testConfig(t, "main")
	s := open(t, cfg)
	if err := s.WriteResult(result.Result{Rule: "../../escape"}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(cfg.ResultsDir())
	if len(entries) != 1 {
		t.Errorf("result must land inside the results dir, entries = %v", entries)
	}
}

func TestPrune(t *testing.T) {
	cfg := testConfig(t, "main")
	s := open(t, cfg)
	for _, rule := range []string{"keep", "gone"} {
		if err := s.WriteResult(result.Result{Rule: rule}); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveState(rule, state.State{State: state.Pass}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Prune([]string{"keep"}); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{cfg.ResultsDir(), cfg.StateDir()} {
		if _, err := os.Stat(filepath.Join(dir, "keep.json")); err != nil {
			t.Errorf("%s/keep.json must stay: %v", dir, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "gone.json")); !os.IsNotExist(err) {
			t.Errorf("%s/gone.json must be removed", dir)
		}
	}
	if _, err := os.Stat(filepath.Join(cfg.StateDir(), ".owner")); err != nil {
		unlock, lerr := s.Lock()
		if lerr != nil {
			t.Fatal(lerr)
		}
		unlock()
	}
	if err := s.Prune([]string{"keep"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lock(); err != nil {
		t.Errorf("prune must not remove lock/owner files: %v", err)
	}
}
