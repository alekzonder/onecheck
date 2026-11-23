package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupWritesFileAtLevel(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sub", "dir", "onecheck.log")
	var stderr bytes.Buffer
	log, closer, err := Setup(file, "info", "text", &stderr)
	if err != nil {
		t.Fatal(err)
	}
	log.Debug("debug-msg")
	log.Info("info-msg", "rule", "api")
	log.Warn("warn-msg")
	closer.Close()

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "debug-msg") || !strings.Contains(text, "info-msg") || !strings.Contains(text, "rule=api") || !strings.Contains(text, "warn-msg") {
		t.Errorf("log file:\n%s", text)
	}
	if strings.Contains(stderr.String(), "info-msg") || !strings.Contains(stderr.String(), "warn-msg") {
		t.Errorf("stderr must get warn and above only:\n%s", stderr.String())
	}
}

func TestSetupJSONFormat(t *testing.T) {
	file := filepath.Join(t.TempDir(), "onecheck.log")
	log, closer, err := Setup(file, "debug", "json", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	log.Debug("hello", "n", 1)
	closer.Close()
	data, _ := os.ReadFile(file)
	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &line); err != nil {
		t.Fatalf("log line is not JSON: %v: %s", err, data)
	}
	if line["msg"] != "hello" || line["level"] != "DEBUG" || line["n"] != float64(1) {
		t.Errorf("line = %v", line)
	}
}

func TestSetupUnwritableFileFallsBackToStderr(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	log, closer, err := Setup(filepath.Join(blocker, "onecheck.log"), "info", "text", &stderr)
	if err == nil {
		t.Error("want an error describing the log file problem")
	}
	if log == nil || closer == nil {
		t.Fatal("a usable logger must still be returned")
	}
	log.Info("still-logged")
	closer.Close()
	if !strings.Contains(stderr.String(), "still-logged") {
		t.Errorf("without a log file everything at the level goes to stderr:\n%s", stderr.String())
	}
}

func TestSetupBadLevel(t *testing.T) {
	if _, _, err := Setup(filepath.Join(t.TempDir(), "x.log"), "loud", "text", &bytes.Buffer{}); err == nil {
		t.Error("unknown level must be an error")
	}
}
