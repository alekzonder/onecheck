package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alekzonder/onecheck/pkg/module"
)

// fakeSystemctl puts a systemctl script first in PATH. Each call appends its
// arguments and $XDG_RUNTIME_DIR as one line to the returned log file and
// prints the fixture of the unit argument; a unit without a fixture makes it
// fail like systemctl does without a bus.
func fakeSystemctl(t *testing.T, fixtures map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	fmt.Fprintf(&script, "echo \"$* XDG=$XDG_RUNTIME_DIR\" >> %q\n", log)
	script.WriteString(`for a in "$@"; do case "$a" in` + "\n")
	for unit, out := range fixtures {
		fixture := filepath.Join(dir, unit+".out")
		if err := os.WriteFile(fixture, []byte(out), 0o600); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&script, "  %q) cat %q; exit 0;;\n", unit, fixture)
	}
	script.WriteString("esac; done\necho 'Failed to connect to bus: No medium found' >&2\nexit 1\n")
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script.String()), 0o700); err != nil { //nolint:gosec // test script must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func calls(t *testing.T, log string) []string {
	t.Helper()
	b, err := os.ReadFile(log) //nolint:gosec // path from t.TempDir
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func run(t *testing.T, params map[string]any, fields ...string) (map[string]any, error) {
	t.Helper()
	raw, _ := json.Marshal(params)
	return collect(context.Background(), module.Request{Params: raw, Fields: fields})
}

const nginxShow = "Id=nginx.service\nLoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\nResult=success\nNRestarts=0\nMainPID=42\nExecMainStatus=0\nMemoryCurrent=1073152\n"

func TestCollectServiceArgs(t *testing.T) {
	log := fakeSystemctl(t, map[string]string{"nginx": nginxShow})
	f, err := run(t, map[string]any{"unit": "nginx"})
	if err != nil {
		t.Fatal(err)
	}
	wantFields(t, f, map[string]any{"active_state": "active", "restarts": int64(0), "main_pid": int64(42)})
	wantAbsent(t, f, "properties")
	c := calls(t, log)
	if len(c) != 1 || !strings.HasPrefix(c[0], "show -p ") || !strings.Contains(c[0], " -- nginx ") || strings.Contains(c[0], "--user") {
		t.Fatalf("calls = %q", c)
	}
	for _, p := range []string{"ActiveState", "LastTriggerUSec", "NRestarts"} {
		if !strings.Contains(c[0], p) {
			t.Errorf("property list lacks %s: %q", p, c[0])
		}
	}
}

func TestCollectUserFlag(t *testing.T) {
	log := fakeSystemctl(t, map[string]string{"app.service": "Id=app.service\nActiveState=active\n"})
	t.Setenv("XDG_RUNTIME_DIR", "")
	_ = os.Unsetenv("XDG_RUNTIME_DIR")
	if _, err := run(t, map[string]any{"unit": "app.service", "user": true}); err != nil {
		t.Fatal(err)
	}
	c := calls(t, log)
	if !strings.HasPrefix(c[0], "--user show -p ") || !strings.Contains(c[0], " -- app.service ") {
		t.Errorf("call = %q", c[0])
	}
	if want := "XDG=/run/user/" + strconv.Itoa(os.Getuid()); !strings.HasSuffix(c[0], want) {
		t.Errorf("call = %q, want suffix %q", c[0], want)
	}
}

func TestCollectUserKeepsRuntimeDir(t *testing.T) {
	log := fakeSystemctl(t, map[string]string{"app.service": "Id=app.service\n"})
	t.Setenv("XDG_RUNTIME_DIR", "/custom")
	if _, err := run(t, map[string]any{"unit": "app.service", "user": true}); err != nil {
		t.Fatal(err)
	}
	if c := calls(t, log); !strings.HasSuffix(c[0], "XDG=/custom") {
		t.Errorf("call = %q", c[0])
	}
}

const timerShow = "Id=backup.timer\nActiveState=active\nUnit=backup.service\nLastTriggerUSec=n/a\n"

func TestCollectTimerReadsActivatedUnit(t *testing.T) {
	log := fakeSystemctl(t, map[string]string{
		"backup.timer":   timerShow,
		"backup.service": "Result=exit-code\nExecMainStatus=2\nActiveState=inactive\n",
	})
	f, err := run(t, map[string]any{"unit": "backup.timer"})
	if err != nil {
		t.Fatal(err)
	}
	wantFields(t, f, map[string]any{"service": "backup.service", "service_result": "exit-code", "service_exit_status": int64(2)})
	c := calls(t, log)
	if len(c) != 2 || !strings.Contains(c[1], " -- backup.service ") {
		t.Errorf("calls = %q", c)
	}
}

func TestCollectTimerSkipsActivatedWhenNotWanted(t *testing.T) {
	log := fakeSystemctl(t, map[string]string{"backup.timer": timerShow})
	if _, err := run(t, map[string]any{"unit": "backup.timer"}, "idle_age"); err != nil {
		t.Fatal(err)
	}
	if c := calls(t, log); len(c) != 1 {
		t.Errorf("calls = %q, want only the timer", c)
	}
}

func TestCollectProperties(t *testing.T) {
	log := fakeSystemctl(t, map[string]string{"nginx.service": nginxShow})
	f, err := run(t, map[string]any{"unit": "nginx.service", "properties": []string{"MemoryCurrent", "Nope"}})
	if err != nil {
		t.Fatal(err)
	}
	props, _ := f["properties"].(map[string]any)
	if len(props) != 1 || props["MemoryCurrent"] != int64(1073152) {
		t.Errorf("properties = %#v", f["properties"])
	}
	if c := calls(t, log); !strings.Contains(c[0], "MemoryCurrent") || !strings.Contains(c[0], "Nope") {
		t.Errorf("call = %q", c[0])
	}
}

func TestCollectSystemctlFailsIsError(t *testing.T) {
	fakeSystemctl(t, nil)
	_, err := run(t, map[string]any{"unit": "app.service", "user": true})
	if err == nil || !strings.Contains(err.Error(), "Failed to connect to bus") {
		t.Errorf("err = %v", err)
	}
}

func TestCollectNoSystemctlIsError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := run(t, map[string]any{"unit": "nginx.service"})
	if err == nil || !strings.Contains(err.Error(), "systemd is not available") {
		t.Errorf("err = %v", err)
	}
}

func TestCollectEmptyUnitIsError(t *testing.T) {
	fakeSystemctl(t, nil)
	if _, err := run(t, map[string]any{"unit": ""}); err == nil {
		t.Error("empty unit must be an error")
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
	want := map[string]module.FieldType{
		"load_state": module.TypeString, "active_state": module.TypeString, "sub_state": module.TypeString,
		"unit_file_state": module.TypeString, "result": module.TypeString, "description": module.TypeString,
		"state_age": module.TypeDuration, "restarts": module.TypeNumber, "main_pid": module.TypeNumber,
		"exit_status": module.TypeNumber, "service": module.TypeString, "last_trigger_age": module.TypeDuration,
		"idle_age": module.TypeDuration, "next_trigger_in": module.TypeDuration, "service_result": module.TypeString,
		"service_exit_status": module.TypeNumber, "service_active_state": module.TypeString, "properties": module.TypeObject,
	}
	if len(d.Fields) != len(want) {
		t.Errorf("describe has %d fields, want %d", len(d.Fields), len(want))
	}
	for k, typ := range want {
		if d.Fields[k].Type != typ {
			t.Errorf("%s type = %q, want %q", k, d.Fields[k].Type, typ)
		}
	}
}

func TestCollectOptionLikeUnitIsNotAnOption(t *testing.T) {
	log := fakeSystemctl(t, map[string]string{"-foo.service": "Id=-foo.service\nLoadState=not-found\n"})
	f, err := run(t, map[string]any{"unit": "-foo.service"})
	if err != nil {
		t.Fatal(err)
	}
	wantFields(t, f, map[string]any{"load_state": "not-found"})
	if c := calls(t, log); !strings.Contains(c[0], " -- -foo.service ") {
		t.Errorf("call = %q, want the unit after --", c[0])
	}
}

func TestCollectGlobIsError(t *testing.T) {
	log := fakeSystemctl(t, nil)
	for _, u := range []string{"*.service", "nginx?", "app[12].service"} {
		_, err := run(t, map[string]any{"unit": u})
		if err == nil || !strings.Contains(err.Error(), "pattern") {
			t.Errorf("unit %q: err = %v", u, err)
		}
	}
	if _, err := os.Stat(log); err == nil {
		t.Error("systemctl must not be called for a pattern")
	}
}

func TestCollectNoIdIsError(t *testing.T) {
	fakeSystemctl(t, map[string]string{"--all": ""})
	if _, err := run(t, map[string]any{"unit": "--all"}); err == nil {
		t.Error("output without Id must be an error")
	}
}

func TestCollectSeveralUnitsIsError(t *testing.T) {
	fakeSystemctl(t, map[string]string{"a.service": "Id=a.service\n\nId=b.service\n"})
	if _, err := run(t, map[string]any{"unit": "a.service"}); err == nil {
		t.Error("output of several units must be an error")
	}
}

func TestCollectPropertyNameWithCommaIsError(t *testing.T) {
	fakeSystemctl(t, map[string]string{"a.service": "Id=a.service\n"})
	if _, err := run(t, map[string]any{"unit": "a.service", "properties": []string{"A,B"}}); err == nil {
		t.Error("a property name with a comma must be an error")
	}
}
