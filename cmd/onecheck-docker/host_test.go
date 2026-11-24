package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type hostEnv struct {
	vars   map[string]string
	home   string
	goos   string
	exists map[string]bool
}

func (h hostEnv) resolve(param string) (endpoint, error) {
	return resolveHost(param, func(k string) string { return h.vars[k] }, h.home, h.goos,
		func(p string) bool { return h.exists[p] })
}

func newHostEnv(t *testing.T, goos string) hostEnv {
	return hostEnv{vars: map[string]string{}, home: t.TempDir(), goos: goos, exists: map[string]bool{}}
}

func writeContext(t *testing.T, configDir, name, host string) {
	t.Helper()
	sum := sha256.Sum256([]byte(name))
	dir := filepath.Join(configDir, "contexts", "meta", hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := `{"Name":"` + name + `","Endpoints":{"docker":{"Host":"` + host + `"}}}`
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeConfig(t *testing.T, configDir, current string) {
	t.Helper()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"currentContext":"`+current+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func wantEndpoint(t *testing.T, got endpoint, err error, addr, source string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if got.addr != addr || got.source != source {
		t.Errorf("endpoint = %+v, want %s from %s", got, addr, source)
	}
}

func TestResolveParamWins(t *testing.T) {
	h := newHostEnv(t, "linux")
	h.vars["DOCKER_HOST"] = "unix:///env.sock"
	h.exists["/var/run/docker.sock"] = true
	ep, err := h.resolve("tcp://10.0.0.5:2375")
	wantEndpoint(t, ep, err, "tcp://10.0.0.5:2375", "params.host")
}

func TestResolveDockerHost(t *testing.T) {
	h := newHostEnv(t, "linux")
	h.vars["DOCKER_HOST"] = "unix:///env.sock"
	ep, err := h.resolve("")
	wantEndpoint(t, ep, err, "unix:///env.sock", "DOCKER_HOST")
}

func TestResolveContextFromConfig(t *testing.T) {
	h := newHostEnv(t, "darwin")
	cfg := filepath.Join(h.home, ".docker")
	writeConfig(t, cfg, "colima")
	writeContext(t, cfg, "colima", "unix:///Users/me/.colima/default/docker.sock")
	h.exists["/var/run/docker.sock"] = true
	ep, err := h.resolve("")
	wantEndpoint(t, ep, err, "unix:///Users/me/.colima/default/docker.sock", "docker context colima")
}

func TestResolveContextFromEnv(t *testing.T) {
	h := newHostEnv(t, "linux")
	cfg := filepath.Join(t.TempDir(), "cfg")
	h.vars["DOCKER_CONFIG"] = cfg
	h.vars["DOCKER_CONTEXT"] = "remote"
	writeConfig(t, cfg, "other")
	writeContext(t, cfg, "remote", "tcp://box:2375")
	ep, err := h.resolve("")
	wantEndpoint(t, ep, err, "tcp://box:2375", "docker context remote")
}

func TestResolveMissingContextIsError(t *testing.T) {
	h := newHostEnv(t, "linux")
	h.vars["DOCKER_CONTEXT"] = "gone"
	if _, err := h.resolve(""); err == nil || !strings.Contains(err.Error(), "gone") {
		t.Errorf("err = %v", err)
	}
}

func TestResolveDefaultContextFallsBack(t *testing.T) {
	h := newHostEnv(t, "linux")
	writeConfig(t, filepath.Join(h.home, ".docker"), "default")
	h.exists["/var/run/docker.sock"] = true
	ep, err := h.resolve("")
	wantEndpoint(t, ep, err, "unix:///var/run/docker.sock", "default socket")
}

func TestResolveRootlessSocket(t *testing.T) {
	h := newHostEnv(t, "linux")
	h.vars["XDG_RUNTIME_DIR"] = "/run/user/1000"
	h.exists["/run/user/1000/docker.sock"] = true
	h.exists["/run/podman/podman.sock"] = true
	ep, err := h.resolve("")
	wantEndpoint(t, ep, err, "unix:///run/user/1000/docker.sock", "default socket")
}

func TestResolveDarwinSockets(t *testing.T) {
	h := newHostEnv(t, "darwin")
	orb := filepath.Join(h.home, ".orbstack/run/docker.sock")
	h.exists[orb] = true
	ep, err := h.resolve("")
	wantEndpoint(t, ep, err, "unix://"+orb, "default socket")
}

func TestResolveDarwinSocketsOnlyOnDarwin(t *testing.T) {
	h := newHostEnv(t, "linux")
	h.exists[filepath.Join(h.home, ".docker/run/docker.sock")] = true
	if _, err := h.resolve(""); err == nil {
		t.Error("macOS sockets must not be tried on linux")
	}
}

func TestResolveNothingFound(t *testing.T) {
	h := newHostEnv(t, "linux")
	h.vars["XDG_RUNTIME_DIR"] = "/run/user/1000"
	_, err := h.resolve("")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, p := range []string{"/var/run/docker.sock", "/run/user/1000/docker.sock", "/run/podman/podman.sock", "DOCKER_HOST"} {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error %q does not mention %s", err, p)
		}
	}
}

func TestResolveUnsupportedScheme(t *testing.T) {
	h := newHostEnv(t, "linux")
	for _, host := range []string{"ssh://me@box", "npipe:////./pipe/docker_engine", "unix://", "tcp://", "box:2375"} {
		if _, err := h.resolve(host); err == nil {
			t.Errorf("%q must be rejected", host)
		}
	}
	h.vars["DOCKER_HOST"] = "ssh://me@box"
	if _, err := h.resolve(""); err == nil || !strings.Contains(err.Error(), "DOCKER_HOST") {
		t.Errorf("err = %v", err)
	}
}
