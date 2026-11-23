package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/onecheck/internal/github"
	"github.com/alekzonder/onecheck/internal/version"
)

// fakeClient serves releases and asset contents from memory.
type fakeClient struct {
	releases map[string][]github.Release
	content  map[string]string // asset URL → content
}

func (f *fakeClient) Releases(_ context.Context, repo string) ([]github.Release, error) {
	rels, ok := f.releases[repo]
	if !ok {
		return nil, fmt.Errorf("repository %s not found", repo)
	}
	return rels, nil
}

func (f *fakeClient) Download(_ context.Context, a github.Asset, w io.Writer) error {
	content, ok := f.content[a.URL]
	if !ok {
		return fmt.Errorf("asset %s not found", a.URL)
	}
	_, err := io.WriteString(w, content)
	return err
}

// script is a tiny shell "module" answering describe.
func script(name, ver string, protocol int) string {
	return fmt.Sprintf("#!/bin/sh\necho '{\"protocol\":%d,\"name\":%q,\"version\":%q,\"fields\":{}}'\n", protocol, name, ver)
}

// addRelease appends a release whose assets are the given name → content.
// With goodSums it also adds a correct checksums.txt.
func (f *fakeClient) addRelease(repo, tag string, prerelease bool, files map[string]string, goodSums bool) {
	if f.releases == nil {
		f.releases, f.content = map[string][]github.Release{}, map[string]string{}
	}
	rel := github.Release{Tag: tag, Prerelease: prerelease}
	var sums strings.Builder
	for name, content := range files {
		url := repo + "/" + tag + "/" + name
		rel.Assets = append(rel.Assets, github.Asset{Name: name, URL: url})
		f.content[url] = content
		sum := sha256.Sum256([]byte(content))
		if !goodSums {
			sum = sha256.Sum256([]byte("something else"))
		}
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	url := repo + "/" + tag + "/checksums.txt"
	rel.Assets = append(rel.Assets, github.Asset{Name: "checksums.txt", URL: url})
	f.content[url] = sums.String()
	// Newest first, like the GitHub API.
	f.releases[repo] = append([]github.Release{rel}, f.releases[repo]...)
}

func standard() *fakeClient {
	f := &fakeClient{}
	f.addRelease("o/r", "v1.0.0", false, map[string]string{
		"onecheck-file_linux_amd64": script("file", "1.0.0", 1),
		"onecheck-http_linux_amd64": script("http", "1.0.0", 1),
		"onecheck-file_linux_arm64": "other platform",
		"onecheck_linux_amd64":      "#!/bin/sh\necho onecheck 1.0.0\n",
	}, true)
	f.addRelease("o/r", "v1.1.0", false, map[string]string{
		"onecheck-file_linux_amd64": script("file", "1.1.0", 1),
		"onecheck-http_linux_amd64": script("http", "1.1.0", 1),
		"onecheck_linux_amd64":      "#!/bin/sh\necho onecheck 1.1.0\n",
	}, true)
	f.addRelease("o/r", "v2.0.0-rc1", true, map[string]string{
		"onecheck-file_linux_amd64": script("file", "2.0.0-rc1", 1),
	}, true)
	return f
}

func manager(t *testing.T, c github.Client) *Manager {
	t.Helper()
	return &Manager{Client: c, Dir: filepath.Join(t.TempDir(), "modules"), OS: "linux", Arch: "amd64"}
}

func files(t *testing.T, dir string) string {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return strings.Join(names, " ")
}

func TestSearchMatchesPlatformAssets(t *testing.T) {
	m := manager(t, standard())
	found, err := m.Search(context.Background(), []string{"o/r"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range found {
		got = append(got, f.Tag+":"+f.Name)
	}
	want := "v2.0.0-rc1:file v1.1.0:file v1.1.0:http v1.0.0:file v1.0.0:http"
	if strings.Join(got, " ") != want {
		t.Errorf("found = %v, want %s", got, want)
	}
	if _, err := m.Search(context.Background(), []string{"o/none"}); err == nil {
		t.Error("an unknown repo must be an error")
	}
}

func TestInstallLatestAll(t *testing.T) {
	m := manager(t, standard())
	found, err := m.Install(context.Background(), "o/r", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || found[0].Tag != "v1.1.0" {
		t.Fatalf("must install every module of the latest stable release: %+v", found)
	}
	if got := files(t, m.Dir); got != "installed.json onecheck-file onecheck-http" {
		t.Errorf("files: %s", got)
	}
	info, err := os.Stat(filepath.Join(m.Dir, "onecheck-file"))
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("module must be executable: %v %v", info, err)
	}
	inst, err := m.Installed()
	if err != nil {
		t.Fatal(err)
	}
	if inst["file"].Repo != "o/r" || inst["file"].Version != "1.1.0" || len(inst["file"].SHA256) != 64 {
		t.Errorf("installed = %+v", inst)
	}
}

func TestInstallPinnedVersion(t *testing.T) {
	m := manager(t, standard())
	for _, spec := range []string{"file@1.0.0", "file@v1.0.0"} {
		found, err := m.Install(context.Background(), "o/r", []string{spec})
		if err != nil || len(found) != 1 || found[0].Tag != "v1.0.0" {
			t.Fatalf("%s: %+v %v", spec, found, err)
		}
	}
	if got := files(t, m.Dir); got != "installed.json onecheck-file" {
		t.Errorf("files: %s", got)
	}
	if _, err := m.Install(context.Background(), "o/r", []string{"file@7.7.7"}); err == nil {
		t.Error("an unknown version must be an error")
	}
	if _, err := m.Install(context.Background(), "o/r", []string{"nosuch"}); err == nil || !strings.Contains(err.Error(), "nosuch") {
		t.Errorf("an unknown module must be an error naming it, got %v", err)
	}
}

func TestInstallChecksumMismatch(t *testing.T) {
	f := &fakeClient{}
	f.addRelease("o/r", "v1.0.0", false, map[string]string{"onecheck-file_linux_amd64": script("file", "1.0.0", 1)}, false)
	m := manager(t, f)
	_, err := m.Install(context.Background(), "o/r", nil)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if got := files(t, m.Dir); got != "" {
		t.Errorf("nothing may be left behind: %s", got)
	}
}

func TestInstallMissingChecksums(t *testing.T) {
	f := &fakeClient{releases: map[string][]github.Release{"o/r": {{Tag: "v1.0.0", Assets: []github.Asset{
		{Name: "onecheck-file_linux_amd64", URL: "u"}}}}}, content: map[string]string{"u": script("file", "1.0.0", 1)}}
	m := manager(t, f)
	if _, err := m.Install(context.Background(), "o/r", nil); err == nil || !strings.Contains(err.Error(), "checksums.txt") {
		t.Errorf("a release without checksums.txt must be refused, got %v", err)
	}
}

func TestInstallIncompatibleProtocol(t *testing.T) {
	good := standard()
	m := manager(t, good)
	if _, err := m.Install(context.Background(), "o/r", []string{"file"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(m.Dir, "onecheck-file"))

	good.addRelease("o/r", "v3.0.0", false, map[string]string{"onecheck-file_linux_amd64": script("file", "3.0.0", 99)}, true)
	_, err := m.Install(context.Background(), "o/r", []string{"file"})
	if err == nil || !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("err = %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(m.Dir, "onecheck-file"))
	if string(before) != string(after) {
		t.Error("the installed module must stay untouched")
	}
	if got := files(t, m.Dir); got != "installed.json onecheck-file" {
		t.Errorf("no temp file may be left: %s", got)
	}
}

func TestInstallWrongName(t *testing.T) {
	f := &fakeClient{}
	f.addRelease("o/r", "v1.0.0", false, map[string]string{"onecheck-file_linux_amd64": script("impostor", "1.0.0", 1)}, true)
	m := manager(t, f)
	if _, err := m.Install(context.Background(), "o/r", nil); err == nil || !strings.Contains(err.Error(), "impostor") {
		t.Errorf("err = %v", err)
	}
}

func TestUpdate(t *testing.T) {
	c := standard()
	m := manager(t, c)
	if _, err := m.Install(context.Background(), "o/r", []string{"file@1.0.0", "http@1.0.0"}); err != nil {
		t.Fatal(err)
	}
	updated, skipped, err := m.Update(context.Background(), []string{"file", "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0].Name != "file" || updated[0].Tag != "v1.1.0" {
		t.Errorf("updated = %+v", updated)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], "manual") {
		t.Errorf("skipped = %v", skipped)
	}
	inst, _ := m.Installed()
	if inst["file"].Version != "1.1.0" || inst["http"].Version != "1.0.0" {
		t.Errorf("installed = %+v", inst)
	}
	updated, _, err = m.Update(context.Background(), nil)
	if err != nil || len(updated) != 1 || updated[0].Name != "http" {
		t.Errorf("update without names must update everything outdated: %+v %v", updated, err)
	}
}

func TestUpdateUpToDateAndIncompatible(t *testing.T) {
	c := standard()
	m := manager(t, c)
	if _, err := m.Install(context.Background(), "o/r", []string{"file"}); err != nil {
		t.Fatal(err)
	}
	updated, skipped, err := m.Update(context.Background(), nil)
	if err != nil || len(updated) != 0 || len(skipped) != 1 || !strings.Contains(skipped[0], "up to date") {
		t.Errorf("updated=%v skipped=%v err=%v", updated, skipped, err)
	}
	c.addRelease("o/r", "v3.0.0", false, map[string]string{"onecheck-file_linux_amd64": script("file", "3.0.0", 99)}, true)
	updated, skipped, err = m.Update(context.Background(), nil)
	if err != nil || len(updated) != 0 || len(skipped) != 1 || !strings.Contains(skipped[0], "protocol") {
		t.Errorf("an incompatible release is skipped, not fatal: updated=%v skipped=%v err=%v", updated, skipped, err)
	}
}

func TestRemove(t *testing.T) {
	m := manager(t, standard())
	if _, err := m.Install(context.Background(), "o/r", nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove([]string{"file"}); err != nil {
		t.Fatal(err)
	}
	if got := files(t, m.Dir); got != "installed.json onecheck-http" {
		t.Errorf("files: %s", got)
	}
	inst, _ := m.Installed()
	if _, still := inst["file"]; still {
		t.Error("the manifest entry must be removed")
	}
	if err := m.Remove([]string{"file"}); err == nil {
		t.Error("removing a module that is not installed must be an error")
	}
	if err := m.Remove([]string{"../escape"}); err == nil {
		t.Error("a module name with a path must be rejected")
	}
}

func TestSelfUpdate(t *testing.T) {
	c := standard()
	exe := filepath.Join(t.TempDir(), "onecheck")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	newVer, err := SelfUpdate(context.Background(), c, "o/r", "", exe, "linux", "amd64")
	if err != nil || newVer != "1.1.0" {
		t.Fatalf("newVer=%q err=%v", newVer, err)
	}
	data, _ := os.ReadFile(exe)
	if !strings.Contains(string(data), "onecheck 1.1.0") {
		t.Errorf("exe = %q", data)
	}
	if info, _ := os.Stat(exe); info.Mode().Perm()&0o111 == 0 {
		t.Error("the new binary must be executable")
	}
	if got := files(t, filepath.Dir(exe)); got != "onecheck" {
		t.Errorf("no temp file may be left: %s", got)
	}
	if v, err := SelfUpdate(context.Background(), c, "o/r", "1.0.0", exe, "linux", "amd64"); err != nil || v != "1.0.0" {
		t.Errorf("pinned version: %q %v", v, err)
	}

	old := version.Version
	version.Version = "1.1.0"
	defer func() { version.Version = old }()
	if v, err := SelfUpdate(context.Background(), c, "o/r", "", exe, "linux", "amd64"); err != nil || v != "" {
		t.Errorf("already at the latest version must be a no-op: %q %v", v, err)
	}
}

func TestSelfUpdateChecksumMismatch(t *testing.T) {
	f := &fakeClient{}
	f.addRelease("o/r", "v1.0.0", false, map[string]string{"onecheck_linux_amd64": "#!/bin/sh\nexit 0\n"}, false)
	exe := filepath.Join(t.TempDir(), "onecheck")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := SelfUpdate(context.Background(), f, "o/r", "", exe, "linux", "amd64"); err == nil {
		t.Fatal("want a checksum error")
	}
	if data, _ := os.ReadFile(exe); string(data) != "old binary" {
		t.Error("the current binary must stay untouched")
	}
	if got := files(t, filepath.Dir(exe)); got != "onecheck" {
		t.Errorf("no temp file may be left: %s", got)
	}
}

func TestSelfUpdateBrokenBinary(t *testing.T) {
	f := &fakeClient{}
	f.addRelease("o/r", "v1.0.0", false, map[string]string{"onecheck_linux_amd64": "#!/bin/sh\nexit 1\n"}, true)
	exe := filepath.Join(t.TempDir(), "onecheck")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := SelfUpdate(context.Background(), f, "o/r", "", exe, "linux", "amd64"); err == nil {
		t.Fatal("a binary that cannot run must not be installed")
	}
	if data, _ := os.ReadFile(exe); string(data) != "old binary" {
		t.Error("the current binary must stay untouched")
	}
}

func TestUpdateChecksumMismatchIsAnError(t *testing.T) {
	c := standard()
	m := manager(t, c)
	if _, err := m.Install(context.Background(), "o/r", []string{"file"}); err != nil {
		t.Fatal(err)
	}
	c.addRelease("o/r", "v1.2.0", false, map[string]string{"onecheck-file_linux_amd64": script("file", "1.2.0", 1)}, false)
	_, _, err := m.Update(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("a checksum mismatch during update must fail the command, got %v", err)
	}
	inst, _ := m.Installed()
	if inst["file"].Version != "1.1.0" {
		t.Errorf("the installed module must stay: %+v", inst)
	}
}
