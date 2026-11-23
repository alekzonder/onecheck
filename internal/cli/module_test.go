package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeGitHub serves one repository "o/r" with the given releases
// (tag → asset name → content) through the GitHub API shape.
func fakeGitHub(t *testing.T, tags []string, releases map[string]map[string]string) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	type asset struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	type release struct {
		Tag    string  `json:"tag_name"`
		Assets []asset `json:"assets"`
	}
	var list []release
	for _, tag := range tags {
		files := releases[tag]
		var sums strings.Builder
		for name, content := range files {
			sum := sha256.Sum256([]byte(content))
			fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
		}
		files["checksums.txt"] = sums.String()
		rel := release{Tag: tag}
		for name, content := range files {
			path := "/assets/" + tag + "/" + name
			rel.Assets = append(rel.Assets, asset{Name: name, URL: srv.URL + path})
			mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(content)) })
		}
		list = append(list, rel)
	}
	mux.HandleFunc("/repos/o/r/releases", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(list)
	})
	t.Setenv("ONECHECK_GITHUB_API", srv.URL)
	t.Setenv("GITHUB_TOKEN", "")
}

func moduleScript(name, ver string) string {
	return fmt.Sprintf("#!/bin/sh\necho '{\"protocol\":1,\"name\":%q,\"version\":%q,\"fields\":{}}'\n", name, ver)
}

func TestModuleInstallSearchUpdateRemove(t *testing.T) {
	plat := "_" + runtime.GOOS + "_" + runtime.GOARCH
	fakeGitHub(t, []string{"v1.1.0", "v1.0.0"}, map[string]map[string]string{
		"v1.1.0": {"onecheck-demo" + plat: moduleScript("demo", "1.1.0"), "onecheck-extra" + plat: moduleScript("extra", "1.1.0")},
		"v1.0.0": {"onecheck-demo" + plat: moduleScript("demo", "1.0.0")},
	})
	dir := filepath.Join(t.TempDir(), "mods")

	code, out, errOut := execute("module", "search", "o/r", "--modules-dir", dir)
	if code != 0 || !strings.Contains(out, "v1.1.0") || !strings.Contains(out, "extra") || !strings.Contains(out, "v1.0.0") {
		t.Fatalf("search: code=%d out=%q err=%q", code, out, errOut)
	}
	code, out, errOut = execute("module", "install", "o/r", "demo@1.0.0", "--modules-dir", dir)
	if code != 0 || !strings.Contains(out, "installed demo 1.0.0") {
		t.Fatalf("install: code=%d out=%q err=%q", code, out, errOut)
	}
	code, out, _ = execute("module", "list", "--modules-dir", dir)
	if code != 0 || !strings.Contains(out, "demo") || !strings.Contains(out, "1.0.0") || !strings.Contains(out, "o/r") {
		t.Errorf("list: %q", out)
	}
	code, out, errOut = execute("module", "update", "--modules-dir", dir)
	if code != 0 || !strings.Contains(out, "updated demo to 1.1.0") {
		t.Fatalf("update: code=%d out=%q err=%q", code, out, errOut)
	}
	code, out, _ = execute("module", "update", "--modules-dir", dir)
	if code != 0 || !strings.Contains(out, "up to date") {
		t.Errorf("second update: code=%d out=%q", code, out)
	}
	if code, _, errOut = execute("module", "remove", "demo", "--modules-dir", dir); code != 0 {
		t.Fatalf("remove: code=%d err=%q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "onecheck-demo")); !os.IsNotExist(err) {
		t.Error("the module file must be gone")
	}
	if code, _, _ = execute("module", "install", "o/missing", "--modules-dir", dir); code != 2 {
		t.Errorf("unknown repo: code=%d, want 2", code)
	}
	if code, _, errOut = execute("module", "install", "--modules-dir", dir); code != 2 || !strings.Contains(errOut, "owner/repo") {
		t.Errorf("install without a repo: code=%d err=%q", code, errOut)
	}
}

func TestModuleCommandsNeedADirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ONECHECK_CONFIG", "")
	if _, err := os.Stat("/etc/onecheck/config.yaml"); err == nil {
		t.Skip("a system config exists on this machine")
	}
	code, _, errOut := execute("module", "list")
	if code != 2 || !strings.Contains(errOut, "--modules-dir") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestSelfUpdateCommand(t *testing.T) {
	plat := "_" + runtime.GOOS + "_" + runtime.GOARCH
	fakeGitHub(t, []string{"v9.9.9"}, map[string]map[string]string{
		"v9.9.9": {"onecheck" + plat: "#!/bin/sh\necho onecheck 9.9.9\n"},
	})
	exe := filepath.Join(t.TempDir(), "onecheck")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONECHECK_SELF_UPDATE_REPO", "o/r")
	t.Setenv("ONECHECK_SELF_UPDATE_TARGET", exe)
	code, out, errOut := execute("self-update")
	if code != 0 || !strings.Contains(out, "updated onecheck") || !strings.Contains(out, "9.9.9") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	if data, _ := os.ReadFile(exe); !strings.Contains(string(data), "9.9.9") {
		t.Errorf("exe = %q", data)
	}
	if code, _, errOut = execute("self-update", "--version", "0.0.1"); code != 2 || !strings.Contains(errOut, "0.0.1") {
		t.Errorf("unknown version: code=%d err=%q", code, errOut)
	}
}
