package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/onecheck/pkg/module"
)

var allFields = []string{"exit_code", "stdout", "stderr", "duration"}

func run(t *testing.T, params map[string]any) (map[string]any, error) {
	t.Helper()
	raw, _ := json.Marshal(params)
	return collect(context.Background(), module.Request{Params: raw, Fields: allFields, MaxFieldSize: 1024})
}

func TestRunShell(t *testing.T) {
	f, err := run(t, map[string]any{"run": "echo out; echo err >&2"})
	if err != nil {
		t.Fatal(err)
	}
	if f["exit_code"] != 0 || f["stdout"] != "out\n" || f["stderr"] != "err\n" {
		t.Errorf("fields = %v", f)
	}
	if d, ok := f["duration"].(float64); !ok || d < 0 {
		t.Errorf("duration = %v", f["duration"])
	}
}

func TestArgv(t *testing.T) {
	f, err := run(t, map[string]any{"argv": []string{"echo", "a b", "$HOME"}})
	if err != nil {
		t.Fatal(err)
	}
	if f["stdout"] != "a b $HOME\n" {
		t.Errorf("argv must not go through a shell: %q", f["stdout"])
	}
}

func TestExitCodeIsValue(t *testing.T) {
	f, err := run(t, map[string]any{"run": "echo partial; exit 3"})
	if err != nil {
		t.Fatalf("a non-zero exit code is a value, not an error: %v", err)
	}
	if f["exit_code"] != 3 || f["stdout"] != "partial\n" {
		t.Errorf("fields = %v", f)
	}
}

func TestNotFoundIsError(t *testing.T) {
	if _, err := run(t, map[string]any{"argv": []string{"/nonexistent/binary-xyz"}}); err == nil {
		t.Error("a command that cannot start must be an error")
	}
}

func TestEnvCwdStdin(t *testing.T) {
	dir := t.TempDir()
	f, err := run(t, map[string]any{
		"run":   "pwd; echo $OC_TEST_VAR; cat",
		"env":   map[string]string{"OC_TEST_VAR": "from-env"},
		"cwd":   dir,
		"stdin": "from-stdin",
	})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := f["stdout"].(string)
	if !strings.HasSuffix(strings.Split(out, "\n")[0], dir[strings.LastIndex(dir, "/"):]) || !strings.Contains(out, "from-env\nfrom-stdin") {
		t.Errorf("stdout = %q", out)
	}
}

func TestRunXorArgv(t *testing.T) {
	bad := []map[string]any{
		{},
		{"run": "true", "argv": []string{"true"}},
		{"argv": []string{}},
		{"run": "true", "bogus": 1},
	}
	for _, params := range bad {
		if _, err := run(t, params); err == nil {
			t.Errorf("params %v must be rejected", params)
		}
	}
}

func TestOutputLimit(t *testing.T) {
	f, err := run(t, map[string]any{"run": "head -c 5000 /dev/zero | tr '\\0' a"})
	if err != nil {
		t.Fatal(err)
	}
	if out, _ := f["stdout"].(string); len(out) != 1025 {
		t.Errorf("stdout is %d bytes; want limit+1", len(out))
	}
}

func TestCancelKillsChildren(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"run": "sleep 30 & sleep 30"})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := collect(ctx, module.Request{Params: raw, Fields: allFields, MaxFieldSize: 1024})
	if err == nil {
		t.Error("a timed out command must be an error")
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("collect blocked for %v", time.Since(start))
	}
}

func TestDescriptionCoversFields(t *testing.T) {
	for _, f := range allFields {
		if _, ok := mod.Description.Fields[f]; !ok {
			t.Errorf("field %q is not described", f)
		}
	}
	if !json.Valid(mod.Description.ParamsSchema) {
		t.Error("params_schema is not valid JSON")
	}
}

func TestTimeoutKeepsOutputSoFar(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	raw, _ := json.Marshal(map[string]any{"run": "echo started; echo warn >&2; exec sleep 30"})
	f, err := collect(ctx, module.Request{Params: raw, Fields: allFields, MaxFieldSize: 1024})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
	if f["stdout"] != "started\n" || f["stderr"] != "warn\n" {
		t.Errorf("output written before the timeout was lost: %v", f)
	}
}

func TestOutputFieldsAreMarked(t *testing.T) {
	for name, info := range mod.Description.Fields {
		if want := name == "stdout" || name == "stderr"; info.Output != want {
			t.Errorf("field %s: output = %v, want %v", name, info.Output, want)
		}
	}
}
