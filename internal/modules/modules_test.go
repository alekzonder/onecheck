package modules

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alekzonder/onecheck/pkg/module"
)

var modDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "onecheck-modules-test")
	if err != nil {
		panic(err)
	}
	modDir = dir
	out, err := exec.Command("go", "build", "-o", Path(dir, "fake"), "./testdata/fakemod").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot build fakemod: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func fake(t *testing.T) *Info {
	t.Helper()
	info, err := Describe(context.Background(), Path(modDir, "fake"))
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestPath(t *testing.T) {
	if got := Path("/x/modules", "http"); got != "/x/modules/onecheck-http" {
		t.Errorf("Path = %q", got)
	}
}

func TestDescribe(t *testing.T) {
	info := fake(t)
	if info.Name != "fake" || info.Version != "1.2.3" || info.Protocol != 1 || !info.Compatible() {
		t.Errorf("info = %+v", info)
	}
	if info.Path != Path(modDir, "fake") || info.Fields["echo"].Type != module.TypeString {
		t.Errorf("info = %+v", info)
	}
}

func TestDescribeIncompatible(t *testing.T) {
	t.Setenv("FAKEMOD_MODE", "protocol99")
	info, err := Describe(context.Background(), Path(modDir, "fake"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Compatible() {
		t.Error("protocol 99 must be incompatible")
	}
}

func TestDescribeGarbage(t *testing.T) {
	for _, mode := range []string{"describe-garbage", "describe-fail"} {
		t.Setenv("FAKEMOD_MODE", mode)
		if _, err := Describe(context.Background(), Path(modDir, "fake")); err == nil {
			t.Errorf("%s: want an error", mode)
		}
	}
	if _, err := Describe(context.Background(), filepath.Join(modDir, "onecheck-none")); err == nil {
		t.Error("missing module must be an error")
	}
}

func TestListIgnoresNonModules(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(Path(modDir, "fake"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]os.FileMode{
		"onecheck-fake":      0o755,
		".onecheck-fake.tmp": 0o755,
		"onecheck-noexec":    0o644,
		"installed.json":     0o644,
		"README":             0o755,
	}
	for name, mode := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "onecheck-broken"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	infos, errs := List(context.Background(), dir)
	if len(infos) != 1 || infos[0].Name != "fake" {
		t.Errorf("infos = %+v", infos)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "onecheck-broken") {
		t.Errorf("errs = %v", errs)
	}
}

func TestListMissingDir(t *testing.T) {
	infos, errs := List(context.Background(), filepath.Join(t.TempDir(), "nope"))
	if len(infos) != 0 || len(errs) != 0 {
		t.Errorf("missing dir must be empty: %v %v", infos, errs)
	}
}

func TestCheckOK(t *testing.T) {
	var stderr bytes.Buffer
	req := module.Request{Rule: "my-rule", MaxFieldSize: 1024}
	resp, err := Check(context.Background(), fake(t), req, 5*time.Second, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Fields["echo"] != "my-rule" || resp.Fields["timeout_ms"] != float64(5000) {
		t.Errorf("fields = %v", resp.Fields)
	}
	if !strings.Contains(stderr.String(), "collecting for my-rule") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestCheckNonZeroExit(t *testing.T) {
	info := fake(t)
	t.Setenv("FAKEMOD_MODE", "exit3")
	_, err := Check(context.Background(), info, module.Request{Rule: "r"}, 5*time.Second, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "exit") {
		t.Errorf("err = %v", err)
	}
}

func TestCheckInvalidJSON(t *testing.T) {
	info := fake(t)
	t.Setenv("FAKEMOD_MODE", "garbage")
	_, err := Check(context.Background(), info, module.Request{Rule: "r"}, 5*time.Second, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("err = %v", err)
	}
}

func TestCheckOutputTooLarge(t *testing.T) {
	info := fake(t)
	t.Setenv("FAKEMOD_MODE", "huge")
	_, err := Check(context.Background(), info, module.Request{Rule: "r", MaxFieldSize: 1024}, 10*time.Second, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("err = %v", err)
	}
}

func TestCheckTimeoutKillsGroup(t *testing.T) {
	info := fake(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	t.Setenv("FAKEMOD_MODE", "hang")
	t.Setenv("FAKEMOD_PIDFILE", pidFile)

	start := time.Now()
	_, err := Check(context.Background(), info, module.Request{Rule: "r"}, 300*time.Millisecond, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Check blocked for %v", elapsed)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(raw))
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("child process %d of the module survived the timeout", pid)
}

// alive reports whether the process exists and is not a zombie waiting to be
// reaped by init.
func alive(pid int) bool {
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
