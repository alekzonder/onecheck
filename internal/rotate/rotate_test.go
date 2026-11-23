package rotate

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/onecheck/internal/config"
)

var now = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

func write(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func names(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

func gunzip(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLogsBelowSizeNoop(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "onecheck.log")
	write(t, file, "small", now)
	rotated, err := Logs(file, config.Rotate{MaxSize: 100, Keep: 3}, now)
	if err != nil || rotated {
		t.Errorf("rotated=%v err=%v", rotated, err)
	}
	if names(t, dir) != "onecheck.log" {
		t.Errorf("files: %s", names(t, dir))
	}
	if rotated, err := Logs(filepath.Join(dir, "absent.log"), config.Rotate{MaxSize: 1, Keep: 3}, now); err != nil || rotated {
		t.Errorf("a missing log is nothing to rotate: %v %v", rotated, err)
	}
}

func TestLogsRotateShiftCompress(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "onecheck.log")
	rules := config.Rotate{MaxSize: 10, Keep: 5, Compress: true}
	for i := 1; i <= 3; i++ {
		write(t, file, fmt.Sprintf("generation %d content", i), now)
		rotated, err := Logs(file, rules, now)
		if err != nil || !rotated {
			t.Fatalf("round %d: rotated=%v err=%v", i, rotated, err)
		}
	}
	if got := names(t, dir); got != "onecheck.log.1.gz onecheck.log.2.gz onecheck.log.3.gz" {
		t.Fatalf("files: %s", got)
	}
	if got := gunzip(t, file+".1.gz"); got != "generation 3 content" {
		t.Errorf(".1.gz = %q, want the newest", got)
	}
	if got := gunzip(t, file+".3.gz"); got != "generation 1 content" {
		t.Errorf(".3.gz = %q, want the oldest", got)
	}
}

func TestLogsRotateUncompressed(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "onecheck.log")
	write(t, file, "plain content here", now)
	if _, err := Logs(file, config.Rotate{MaxSize: 5, Keep: 2}, now); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file + ".1")
	if err != nil || string(data) != "plain content here" {
		t.Errorf("onecheck.log.1 = %q, %v", data, err)
	}
}

func TestLogsKeepAndMaxAge(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "onecheck.log")
	old := now.Add(-10 * 24 * time.Hour)
	write(t, file, "tiny", now)
	write(t, file+".1.gz", "a", now.Add(-time.Hour))
	write(t, file+".2.gz", "b", now.Add(-2*time.Hour))
	write(t, file+".3", "c", old)
	write(t, file+".4.gz", "d", now)
	write(t, filepath.Join(dir, "other.log.9"), "unrelated", old)

	rules := config.Rotate{MaxSize: 1 << 20, Keep: 3, MaxAge: config.Duration(7 * 24 * time.Hour)}
	if _, err := Logs(file, rules, now); err != nil {
		t.Fatal(err)
	}
	if got := names(t, dir); got != "onecheck.log onecheck.log.1.gz onecheck.log.2.gz other.log.9" {
		t.Errorf("files: %s (index 3 is too old, index 4 is beyond keep)", got)
	}
}

func event(expires time.Time) string {
	return fmt.Sprintf(`{"id":"x","expires_at":%q}`, expires.Format(time.RFC3339))
}

func TestEventsExpired(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "expired.json"), event(now.Add(-time.Minute)), now)
	write(t, filepath.Join(dir, "fresh.json"), event(now.Add(time.Hour)), now.Add(-100*time.Hour))
	write(t, filepath.Join(dir, "notes.txt"), "keep me", now.Add(-100*time.Hour))
	removed, err := Events(dir, time.Hour, now)
	if err != nil || removed != 1 {
		t.Errorf("removed=%d err=%v", removed, err)
	}
	if got := names(t, dir); got != "fresh.json notes.txt" {
		t.Errorf("files: %s", got)
	}
}

func TestEventsUnparseableOld(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "broken-old.json"), "{garbage", now.Add(-2*time.Hour))
	write(t, filepath.Join(dir, "broken-new.json"), "{garbage", now.Add(-time.Minute))
	write(t, filepath.Join(dir, ".x.json.123.tmp"), "partial", now.Add(-2*time.Hour))
	write(t, filepath.Join(dir, ".y.json.456.tmp"), "partial", now)
	removed, err := Events(dir, time.Hour, now)
	if err != nil || removed != 2 {
		t.Errorf("removed=%d err=%v", removed, err)
	}
	if got := names(t, dir); got != ".y.json.456.tmp broken-new.json" {
		t.Errorf("files: %s", got)
	}
}

func TestEventsMissingDir(t *testing.T) {
	if removed, err := Events(filepath.Join(t.TempDir(), "nope"), time.Hour, now); err != nil || removed != 0 {
		t.Errorf("a missing events dir is nothing to clean: %d %v", removed, err)
	}
}
