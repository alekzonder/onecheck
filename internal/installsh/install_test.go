// Package installsh tests install.sh, the installer in the repository root,
// against a local stand-in for GitHub.
package installsh

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeGitHub serves a "latest release" answer and release downloads.
type fakeGitHub struct {
	latest   string            // tag of the latest release
	releases map[string]string // version -> content of the onecheck asset
	badSum   map[string]bool   // versions whose checksums.txt is wrong
}

func asset() string { return fmt.Sprintf("onecheck_%s_%s", runtime.GOOS, runtime.GOARCH) }

// fakeBinary is a shell script that behaves like onecheck as far as the
// installer cares: it answers "version" and records every call.
func fakeBinary(version string) string {
	return "#!/bin/sh\n" +
		"[ -n \"${CALLS:-}\" ] && echo \"" + version + " $*\" >> \"$CALLS\"\n" +
		"echo \"onecheck " + version + " (protocol 1)\"\n"
}

func (g *fakeGitHub) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/alekzonder/onecheck/releases/latest" {
			_, _ = fmt.Fprintf(w, "{\n  \"id\": 1,\n  \"tag_name\": %q,\n  \"name\": \"x\"\n}\n", g.latest)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/download/"), "/")
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		content, ok := g.releases[strings.TrimPrefix(parts[0], "v")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		switch parts[1] {
		case asset():
			_, _ = w.Write([]byte(content))
		case "checksums.txt":
			sum := sha256.Sum256([]byte(content))
			if g.badSum[strings.TrimPrefix(parts[0], "v")] {
				sum = sha256.Sum256([]byte("something else"))
			}
			_, _ = fmt.Fprintf(w, "%s  onecheck-file_linux_amd64\n%s  %s\n",
				strings.Repeat("0", 64), hex.EncodeToString(sum[:]), asset())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type env struct {
	t     *testing.T
	srv   *httptest.Server
	root  string
	bin   string
	mods  string
	calls string
}

func newEnv(t *testing.T, g *fakeGitHub) *env {
	t.Helper()
	dir := t.TempDir()
	return &env{
		t:     t,
		srv:   g.start(t),
		root:  filepath.Join(dir, "root"),
		bin:   filepath.Join(dir, "bin"),
		mods:  filepath.Join(dir, "modules"),
		calls: filepath.Join(dir, "calls"),
	}
}

// install runs install.sh with the test directories plus extra arguments.
func (e *env) install(args ...string) (string, error) {
	e.t.Helper()
	full := append([]string{"../../install.sh",
		"--root", e.root, "--bin-dir", e.bin, "--modules-dir", e.mods}, args...)
	cmd := exec.Command("sh", full...)
	cmd.Env = append(os.Environ(),
		"ONECHECK_GITHUB_API="+e.srv.URL,
		"ONECHECK_DOWNLOAD_URL="+e.srv.URL+"/download",
		"CALLS="+e.calls,
		"HOME="+e.t.TempDir(),
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (e *env) linkTarget() string {
	e.t.Helper()
	target, err := os.Readlink(filepath.Join(e.bin, "onecheck"))
	if err != nil {
		e.t.Fatalf("no symlink in bin dir: %v", err)
	}
	return target
}

func (e *env) versionPath(v string) string {
	return filepath.Join(e.root, "versions", v, "onecheck")
}

func twoReleases() *fakeGitHub {
	return &fakeGitHub{
		latest: "v1.3.0",
		releases: map[string]string{
			"1.2.0": fakeBinary("1.2.0"),
			"1.3.0": fakeBinary("1.3.0"),
		},
	}
}

func TestInstallsLatestVersionByDefault(t *testing.T) {
	e := newEnv(t, twoReleases())
	out, err := e.install("--no-modules")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	if got := e.linkTarget(); got != e.versionPath("1.3.0") {
		t.Errorf("symlink points to %s, want %s", got, e.versionPath("1.3.0"))
	}
	info, err := os.Stat(e.versionPath("1.3.0"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("binary mode %o, want 755", info.Mode().Perm())
	}
	ran, err := exec.Command(filepath.Join(e.bin, "onecheck"), "version").Output()
	if err != nil || !strings.Contains(string(ran), "onecheck 1.3.0") {
		t.Errorf("installed binary does not run: %v %q", err, ran)
	}
}

func TestInstallsRequestedVersion(t *testing.T) {
	for _, arg := range []string{"1.2.0", "v1.2.0"} {
		e := newEnv(t, twoReleases())
		if out, err := e.install("--no-modules", "--version", arg); err != nil {
			t.Fatalf("--version %s failed: %v\n%s", arg, err, out)
		}
		if got := e.linkTarget(); got != e.versionPath("1.2.0") {
			t.Errorf("--version %s: symlink points to %s", arg, got)
		}
	}
}

func TestSwitchingVersionKeepsTheOldOne(t *testing.T) {
	e := newEnv(t, twoReleases())
	for _, v := range []string{"1.2.0", "1.3.0", "1.2.0"} {
		if out, err := e.install("--no-modules", "--version", v); err != nil {
			t.Fatalf("install %s failed: %v\n%s", v, err, out)
		}
		if got := e.linkTarget(); got != e.versionPath(v) {
			t.Errorf("after installing %s the symlink points to %s", v, got)
		}
	}
	for _, v := range []string{"1.2.0", "1.3.0"} {
		if _, err := os.Stat(e.versionPath(v)); err != nil {
			t.Errorf("version %s is gone: %v", v, err)
		}
	}
}

func TestChecksumMismatchInstallsNothing(t *testing.T) {
	g := twoReleases()
	g.badSum = map[string]bool{"1.3.0": true}
	e := newEnv(t, g)
	if out, err := e.install("--no-modules", "--version", "1.2.0"); err != nil {
		t.Fatalf("install 1.2.0 failed: %v\n%s", err, out)
	}
	out, err := e.install("--no-modules")
	if err == nil {
		t.Fatalf("install with a wrong checksum succeeded:\n%s", out)
	}
	if !strings.Contains(out, "checksum") {
		t.Errorf("error does not mention the checksum:\n%s", out)
	}
	if _, err := os.Stat(e.versionPath("1.3.0")); err == nil {
		t.Error("a binary with a wrong checksum was installed")
	}
	if got := e.linkTarget(); got != e.versionPath("1.2.0") {
		t.Errorf("symlink moved to %s after a failed install", got)
	}
}

func TestUnknownVersionFailsClearly(t *testing.T) {
	e := newEnv(t, twoReleases())
	out, err := e.install("--no-modules", "--version", "9.9.9")
	if err == nil {
		t.Fatalf("install of a missing version succeeded:\n%s", out)
	}
	if !strings.Contains(out, "9.9.9") {
		t.Errorf("error does not name the version:\n%s", out)
	}
	if _, err := os.Lstat(filepath.Join(e.bin, "onecheck")); err == nil {
		t.Error("a symlink was created for a version that does not exist")
	}
}

func TestInstallsBundledModulesOfTheSameVersion(t *testing.T) {
	e := newEnv(t, twoReleases())
	if out, err := e.install("--version", "1.2.0"); err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	calls, err := os.ReadFile(e.calls)
	if err != nil {
		t.Fatal(err)
	}
	want := "1.2.0 module install alekzonder/onecheck file@1.2.0 http@1.2.0 command@1.2.0 disk@1.2.0 systemd@1.2.0 cpu@1.2.0 memory@1.2.0 docker@1.2.0 ports@1.2.0 --modules-dir " + e.mods
	if !strings.Contains(string(calls), want) {
		t.Errorf("modules were not installed.\nwant call: %s\ngot calls:\n%s", want, calls)
	}
}

func TestNoModulesSkipsModuleInstall(t *testing.T) {
	e := newEnv(t, twoReleases())
	if out, err := e.install("--no-modules"); err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	calls, _ := os.ReadFile(e.calls)
	if strings.Contains(string(calls), "module install") {
		t.Errorf("--no-modules still installed modules:\n%s", calls)
	}
}

func TestRefusesToReplaceADirectory(t *testing.T) {
	e := newEnv(t, twoReleases())
	if err := os.MkdirAll(filepath.Join(e.bin, "onecheck"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := e.install("--no-modules")
	if err == nil {
		t.Fatalf("install over a directory succeeded:\n%s", out)
	}
}
