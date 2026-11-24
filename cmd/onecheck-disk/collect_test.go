package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/onecheck/pkg/module"
)

func collectPath(t *testing.T, path string) (map[string]any, error) {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"path": path})
	return collect(context.Background(), module.Request{Params: params})
}

func TestCollectTempDir(t *testing.T) {
	dir := t.TempDir()
	f, err := collectPath(t, dir)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	total, _ := f["total"].(int64)
	free, _ := f["free"].(int64)
	if total <= 0 || free < 0 || free > total {
		t.Errorf("total=%v free=%v", f["total"], f["free"])
	}
	if p, ok := f["used_percent"].(float64); !ok || p < 0 || p > 100 {
		t.Errorf("used_percent = %v", f["used_percent"])
	}
	real, _ := filepath.EvalSymlinks(dir)
	mp, _ := f["mountpoint"].(string)
	if mp == "" || !contains(mp, real) {
		t.Errorf("mountpoint %q does not hold %q", mp, real)
	}
	if f["fstype"] == "" || f["fstype"] == nil {
		t.Errorf("fstype = %v", f["fstype"])
	}
}

func TestCollectFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	f, err := collectPath(t, link)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if f["mountpoint"] == nil {
		t.Errorf("no mountpoint for a symlinked path: %v", f)
	}
}

func TestCollectMissingPathIsError(t *testing.T) {
	_, err := collectPath(t, filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("err = %v, want an error naming the path", err)
	}
}

func TestCollectEmptyPathIsError(t *testing.T) {
	if _, err := collectPath(t, ""); err == nil {
		t.Error("empty path accepted")
	}
}

func TestDescribeListsFields(t *testing.T) {
	want := map[string]module.FieldType{
		"total": module.TypeBytes, "free": module.TypeBytes, "used": module.TypeBytes,
		"used_percent": module.TypeNumber, "inodes_total": module.TypeNumber,
		"inodes_free": module.TypeNumber, "inodes_used": module.TypeNumber,
		"inodes_used_percent": module.TypeNumber, "mountpoint": module.TypeString,
		"device": module.TypeString, "fstype": module.TypeString,
	}
	fields := mod.Description.Fields
	if len(fields) != len(want) {
		t.Errorf("%d fields, want %d", len(fields), len(want))
	}
	for name, typ := range want {
		if fields[name].Type != typ {
			t.Errorf("%s: type %q, want %q", name, fields[name].Type, typ)
		}
	}
	if mod.Description.Name != "disk" {
		t.Errorf("name = %q", mod.Description.Name)
	}
}
