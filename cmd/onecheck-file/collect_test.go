package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/onecheck/pkg/module"
)

var allFields = []string{"exists", "type", "size", "age", "mode", "owner", "group", "content", "count", "target"}

func run(t *testing.T, path string, fields []string, limit int64) map[string]any {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"path": path})
	out, err := collect(context.Background(), module.Request{Params: params, Fields: fields, MaxFieldSize: limit})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	return out
}

func TestFileRegular(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	f := run(t, path, allFields, 1024)
	if f["exists"] != true || f["type"] != "file" || f["size"] != int64(6) || f["mode"] != "0640" || f["content"] != "hello\n" {
		t.Errorf("fields = %v", f)
	}
	age, _ := f["age"].(float64)
	if age < 7190 || age > 7300 {
		t.Errorf("age = %v, want about 7200", f["age"])
	}
	if f["owner"] == "" || f["owner"] == nil || f["group"] == nil {
		t.Errorf("owner/group = %v/%v", f["owner"], f["group"])
	}
	if _, has := f["count"]; has {
		t.Error("count must be absent for a file")
	}
	if _, has := f["target"]; has {
		t.Error("target must be absent for a regular file")
	}
}

func TestFileMissing(t *testing.T) {
	f := run(t, filepath.Join(t.TempDir(), "nope"), allFields, 1024)
	if len(f) != 1 || f["exists"] != false {
		t.Errorf("fields = %v", f)
	}
}

func TestDirSizeCount(t *testing.T) {
	dir := t.TempDir()
	for name, size := range map[string]int{"a": 10, "b": 20, "sub/c": 30, "sub/deep/d": 40} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(strings.Repeat("x", size)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := run(t, dir, allFields, 1024)
	if f["type"] != "dir" || f["size"] != int64(100) || f["count"] != 3 {
		t.Errorf("fields = %v", f)
	}
	if _, has := f["content"]; has {
		t.Error("content must be absent for a directory")
	}
}

func TestSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	f := run(t, link, allFields, 1024)
	if f["exists"] != true || f["type"] != "symlink" || f["target"] != target {
		t.Errorf("fields = %v", f)
	}
	if f["size"] != int64(4) || f["mode"] != "0600" || f["content"] != "data" {
		t.Errorf("fields other than type/target must describe the link target: %v", f)
	}
}

func TestDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "gone"), link); err != nil {
		t.Fatal(err)
	}
	f := run(t, link, allFields, 1024)
	if f["exists"] != false || f["type"] != "symlink" || f["target"] != filepath.Join(dir, "gone") || len(f) != 3 {
		t.Errorf("fields = %v", f)
	}
}

func TestContentLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 5000)), 0o644); err != nil {
		t.Fatal(err)
	}
	f := run(t, path, []string{"content"}, 100)
	content, _ := f["content"].(string)
	if len(content) != 101 {
		t.Errorf("content is %d bytes; want limit+1 so the SDK can flag truncation", len(content))
	}
}

func TestContentNotReadUnlessWanted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	f := run(t, path, []string{"exists", "size"}, 1024)
	if _, has := f["content"]; has {
		t.Error("content must not be read when not requested")
	}
	if f["size"] != int64(1) {
		t.Errorf("fields = %v", f)
	}
}

func TestParamsValidation(t *testing.T) {
	for _, params := range []string{`{}`, `{"path":""}`, `{"path":"/x","extra":1}`, `not json`} {
		if _, err := collect(context.Background(), module.Request{Params: json.RawMessage(params)}); err == nil {
			t.Errorf("params %s must be rejected", params)
		}
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
