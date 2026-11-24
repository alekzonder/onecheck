package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/onecheck/pkg/module"
)

const listJSON = `[
 {"Id":"bbbbbbbbbbbbbbbbbbbb","Names":["/web-1"],"Image":"registry.local/web:3","State":"running","Labels":{"com.docker.compose.project":"shop"}},
 {"Id":"aaaaaaaaaaaaaaaaaaaa","Names":["/nginx"],"Image":"nginx:1.27","State":"running","Labels":{}},
 {"Id":"cccccccccccccccccccc","Names":["/old"],"Image":"busybox","State":"exited","Labels":{"com.docker.compose.project":"tools"}}
]`

var inspectJSON = map[string]string{
	"aaaaaaaaaaaaaaaaaaaa": `{"Id":"aaaaaaaaaaaaaaaaaaaa","Name":"/nginx","RestartCount":2,
	  "State":{"Status":"running","OOMKilled":false,"ExitCode":0,"StartedAt":"2026-11-24T10:00:00Z","Health":{"Status":"healthy"}},
	  "Config":{"Image":"nginx:1.27","Labels":{}}}`,
	"bbbbbbbbbbbbbbbbbbbb": `{"Id":"bbbbbbbbbbbbbbbbbbbb","Name":"/web-1","RestartCount":0,
	  "State":{"Status":"running","OOMKilled":false,"ExitCode":0,"StartedAt":"2026-11-24T11:59:00Z"},
	  "Config":{"Image":"registry.local/web:3","Labels":{"com.docker.compose.project":"shop"}}}`,
	"cccccccccccccccccccc": `{"Id":"cccccccccccccccccccc","Name":"/old","RestartCount":0,
	  "State":{"Status":"exited","OOMKilled":true,"ExitCode":137,"StartedAt":"2026-11-23T00:00:00Z"},
	  "Config":{"Image":"busybox","Labels":{"com.docker.compose.project":"tools"}}}`,
}

type fakeDocker struct {
	mu       sync.Mutex
	requests []string
	listCode int
	gone     map[string]bool
}

func (f *fakeDocker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.RequestURI())
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/containers/json" {
		if f.listCode != 0 {
			w.WriteHeader(f.listCode)
			_, _ = w.Write([]byte(`{"message":"boom from daemon"}`))
			return
		}
		_, _ = w.Write([]byte(listJSON))
		return
	}
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/containers/"), "/json")
	body, ok := inspectJSON[id]
	if !ok || f.gone[id] {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"No such container: ` + id + `"}`))
		return
	}
	_, _ = w.Write([]byte(body))
}

func (f *fakeDocker) inspects() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if r != "/containers/json?all=1" {
			n++
		}
	}
	return n
}

// startFake serves a fake Docker API on a unix socket and points the module
// at it.
func startFake(t *testing.T) *fakeDocker {
	t.Helper()
	f := &fakeDocker{gone: map[string]bool{}}
	sock := filepath.Join(t.TempDir(), "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(f)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	useEndpoint(t, endpoint{addr: "unix://" + sock, source: "test"})
	return f
}

func useEndpoint(t *testing.T, ep endpoint) {
	t.Helper()
	oldResolve, oldNow := resolve, now
	t.Cleanup(func() { resolve, now = oldResolve, oldNow })
	resolve = func(string) (endpoint, error) { return ep, nil }
	now = func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }
}

func run(t *testing.T, params string, fields ...string) (map[string]any, error) {
	t.Helper()
	return collect(context.Background(), module.Request{Params: json.RawMessage(params), Fields: fields})
}

func containers(t *testing.T, f map[string]any) []map[string]any {
	t.Helper()
	list, ok := f["containers"].([]map[string]any)
	if !ok {
		t.Fatalf("containers = %#v", f["containers"])
	}
	return list
}

func names(t *testing.T, f map[string]any) string {
	var out []string
	for _, c := range containers(t, f) {
		out = append(out, c["name"].(string))
	}
	return strings.Join(out, ",")
}

func TestCollectAllContainers(t *testing.T) {
	startFake(t)
	f, err := run(t, `{}`, "containers", "count")
	if err != nil {
		t.Fatal(err)
	}
	if f["count"] != int64(3) || names(t, f) != "nginx,old,web-1" {
		t.Fatalf("count %v, names %s", f["count"], names(t, f))
	}
	nginx := containers(t, f)[0]
	want := map[string]any{
		"name": "nginx", "id": "aaaaaaaaaaaa", "image": "nginx:1.27", "state": "running", "health": "healthy",
		"restart_count": int64(2), "exit_code": int64(0), "oom_killed": false, "uptime_seconds": int64(7200),
	}
	for k, v := range want {
		if nginx[k] != v {
			t.Errorf("%s = %#v, want %#v", k, nginx[k], v)
		}
	}
	for _, c := range containers(t, f) {
		if len(c) != 10 {
			t.Errorf("%s has %d keys, want 10: %v", c["name"], len(c), c)
		}
	}
	old := containers(t, f)[1]
	if old["health"] != "none" || old["exit_code"] != int64(137) || old["oom_killed"] != true || old["uptime_seconds"] != int64(0) {
		t.Errorf("old = %v", old)
	}
	if labels, _ := old["labels"].(map[string]string); labels["com.docker.compose.project"] != "tools" {
		t.Errorf("labels = %#v", old["labels"])
	}
}

func TestCollectFilters(t *testing.T) {
	startFake(t)
	cases := map[string]string{
		`{"name":"nginx"}`:                                  "nginx",
		`{"name":"ngin"}`:                                   "",
		`{"name":"web-*"}`:                                  "web-1",
		`{"label":"com.docker.compose.project"}`:            "old,web-1",
		`{"label":"com.docker.compose.project=shop"}`:       "web-1",
		`{"name":"*","label":"com.docker.compose.project"}`: "old,web-1",
		`{"name":"nginx","label":"com.docker.compose.project=shop"}`: "",
	}
	for params, want := range cases {
		f, err := run(t, params, "containers", "count")
		if err != nil {
			t.Fatalf("%s: %v", params, err)
		}
		if got := names(t, f); got != want {
			t.Errorf("%s: names %q, want %q", params, got, want)
		}
		if f["count"] != int64(len(containers(t, f))) {
			t.Errorf("%s: count %v", params, f["count"])
		}
	}
}

func TestCollectCountOnlySkipsInspect(t *testing.T) {
	fake := startFake(t)
	f, err := run(t, `{}`, "count")
	if err != nil {
		t.Fatal(err)
	}
	if f["count"] != int64(3) || fake.inspects() != 0 {
		t.Errorf("count %v, inspect requests %d", f["count"], fake.inspects())
	}
	if _, has := f["containers"]; has {
		t.Error("containers must not be collected when not asked for")
	}
}

func TestCollectNoFieldsCollectsAll(t *testing.T) {
	startFake(t)
	f, err := run(t, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if names(t, f) != "nginx,old,web-1" {
		t.Errorf("names %s", names(t, f))
	}
}

func TestCollectSkipsRemovedContainer(t *testing.T) {
	fake := startFake(t)
	fake.gone["cccccccccccccccccccc"] = true
	f, err := run(t, `{}`, "containers", "count")
	if err != nil {
		t.Fatal(err)
	}
	if names(t, f) != "nginx,web-1" || f["count"] != int64(2) {
		t.Errorf("names %s count %v", names(t, f), f["count"])
	}
}

func TestCollectDaemonErrorIsError(t *testing.T) {
	fake := startFake(t)
	fake.listCode = http.StatusInternalServerError
	_, err := run(t, `{}`, "count")
	if err == nil || !strings.Contains(err.Error(), "boom from daemon") || !strings.Contains(err.Error(), "(test)") {
		t.Errorf("err = %v", err)
	}
}

func TestCollectUnreachableIsError(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "none.sock")
	useEndpoint(t, endpoint{addr: "unix://" + sock, source: "default socket"})
	_, err := run(t, `{}`, "count")
	if err == nil || !strings.Contains(err.Error(), sock) || !strings.Contains(err.Error(), "default socket") {
		t.Errorf("err = %v", err)
	}
}

func TestCollectPermissionDeniedHint(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	dir := t.TempDir()
	sock := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	if err := os.Chmod(sock, 0); err != nil {
		t.Fatal(err)
	}
	useEndpoint(t, endpoint{addr: "unix://" + sock, source: "default socket"})
	_, err = run(t, `{}`, "count")
	if err == nil || !strings.Contains(err.Error(), "docker group") {
		t.Errorf("err = %v", err)
	}
}

func TestCollectTCPEndpoint(t *testing.T) {
	srv := httptest.NewServer(&fakeDocker{gone: map[string]bool{}})
	t.Cleanup(srv.Close)
	useEndpoint(t, endpoint{addr: "tcp://" + strings.TrimPrefix(srv.URL, "http://"), source: "params.host"})
	f, err := run(t, `{}`, "count")
	if err != nil || f["count"] != int64(3) {
		t.Errorf("f = %v, err = %v", f, err)
	}
}

func TestCollectBadParams(t *testing.T) {
	startFake(t)
	for _, p := range []string{`{"x":1}`, `{"name":"[a"}`} {
		if _, err := run(t, p, "count"); err == nil {
			t.Errorf("%s must be an error", p)
		}
	}
}

func TestCollectCancelled(t *testing.T) {
	startFake(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := collect(ctx, module.Request{Params: json.RawMessage(`{}`)}); err == nil {
		t.Error("cancelled context must be an error")
	}
}

func TestDescribeListsFields(t *testing.T) {
	var out bytes.Buffer
	if code := module.Run(mod, []string{"describe"}, strings.NewReader(""), &out, &out); code != 0 {
		t.Fatalf("describe exit %d: %s", code, out.String())
	}
	var d module.Description
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Fields) != 2 || d.Fields["containers"].Type != module.TypeArray || d.Fields["count"].Type != module.TypeNumber {
		t.Errorf("fields = %+v", d.Fields)
	}
}

func TestCollectUnreachableMessageIsShort(t *testing.T) {
	useEndpoint(t, endpoint{addr: "unix:///nonexistent.sock", source: "params.host"})
	_, err := run(t, `{}`, "count")
	if err == nil || strings.Contains(err.Error(), "http://") {
		t.Errorf("err = %v", err)
	}
}
