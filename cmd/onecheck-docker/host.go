package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// endpoint is a Docker address and where it came from, for error messages.
type endpoint struct {
	addr   string // unix:///path or tcp://host:port
	source string
}

func (e endpoint) String() string { return fmt.Sprintf("docker at %s (%s)", e.addr, e.source) }

// resolveHost finds the Docker address: params.host, DOCKER_HOST, the current
// Docker context, then the first existing well-known socket.
func resolveHost(param string, env func(string) string, home, goos string, exists func(string) bool) (endpoint, error) {
	if param != "" {
		return checked(endpoint{param, "params.host"})
	}
	if env("DOCKER_TLS_VERIFY") != "" {
		return endpoint{}, fmt.Errorf("DOCKER_TLS_VERIFY is set, but TLS connections to Docker are not supported; use a unix socket or plain tcp:// in params.host")
	}
	if h := env("DOCKER_HOST"); h != "" {
		return checked(endpoint{h, "DOCKER_HOST"})
	}
	ep, found, err := fromContext(env, home)
	if err != nil || found {
		return ep, err
	}
	tried := socketCandidates(env, home, goos)
	for _, p := range tried {
		if exists(p) {
			return endpoint{"unix://" + p, "default socket"}, nil
		}
	}
	return endpoint{}, fmt.Errorf("docker not found: no params.host, DOCKER_HOST or docker context, and no socket at %s",
		strings.Join(tried, ", "))
}

func socketCandidates(env func(string) string, home, goos string) []string {
	out := []string{"/var/run/docker.sock"}
	xdg := env("XDG_RUNTIME_DIR")
	if xdg != "" {
		out = append(out, filepath.Join(xdg, "docker.sock"))
	}
	if goos == "darwin" && home != "" {
		out = append(out,
			filepath.Join(home, ".docker/run/docker.sock"),
			filepath.Join(home, ".orbstack/run/docker.sock"),
			filepath.Join(home, ".colima/default/docker.sock"))
	}
	if xdg != "" {
		out = append(out, filepath.Join(xdg, "podman/podman.sock"))
	}
	return append(out, "/run/podman/podman.sock")
}

// fromContext reads the endpoint of the current Docker context, if one other
// than "default" is selected.
func fromContext(env func(string) string, home string) (endpoint, bool, error) {
	dir := env("DOCKER_CONFIG")
	if dir == "" && home != "" {
		dir = filepath.Join(home, ".docker")
	}
	name := env("DOCKER_CONTEXT")
	if name != "" && dir == "" {
		return endpoint{}, false, fmt.Errorf("docker context %q: no home directory or DOCKER_CONFIG to find it in", name)
	}
	if name == "" && dir != "" {
		var cfg struct {
			CurrentContext string `json:"currentContext"`
		}
		data, err := os.ReadFile(filepath.Join(dir, "config.json"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return endpoint{}, false, fmt.Errorf("read docker config: %w", err)
		}
		if err == nil && json.Unmarshal(data, &cfg) != nil {
			return endpoint{}, false, fmt.Errorf("read docker config %s: invalid JSON", filepath.Join(dir, "config.json"))
		}
		name = cfg.CurrentContext
	}
	if name == "" || name == "default" {
		return endpoint{}, false, nil
	}
	sum := sha256.Sum256([]byte(name))
	path := filepath.Join(dir, "contexts", "meta", hex.EncodeToString(sum[:]), "meta.json")
	var meta struct {
		Endpoints map[string]struct {
			Host string `json:"Host"`
		} `json:"Endpoints"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return endpoint{}, false, fmt.Errorf("docker context %q: %w", name, err)
	}
	if err := json.Unmarshal(data, &meta); err != nil || meta.Endpoints["docker"].Host == "" {
		return endpoint{}, false, fmt.Errorf("docker context %q: no docker endpoint in %s", name, path)
	}
	if _, err := os.Stat(filepath.Join(dir, "contexts", "tls", hex.EncodeToString(sum[:]), "docker")); err == nil {
		return endpoint{}, false, fmt.Errorf("docker context %q uses TLS, which is not supported; set params.host to a unix socket or plain tcp:// address", name)
	}
	ep, err := checked(endpoint{meta.Endpoints["docker"].Host, "docker context " + name})
	return ep, true, err
}

// checked rejects addresses the module cannot talk to and normalizes tcp
// ones to host:port (default port 2375, like the docker CLI).
func checked(ep endpoint) (endpoint, error) {
	bad := fmt.Errorf("unsupported docker host %q from %s: use unix:///path or tcp://host:port", ep.addr, ep.source)
	scheme, rest, ok := strings.Cut(ep.addr, "://")
	if !ok || rest == "" {
		return endpoint{}, bad
	}
	switch scheme {
	case "unix":
		return ep, nil
	case "tcp":
		hostport, _, _ := strings.Cut(rest, "/")
		if hostport == "" {
			return endpoint{}, bad
		}
		if _, _, err := net.SplitHostPort(hostport); err != nil {
			hostport = net.JoinHostPort(hostport, "2375")
		}
		ep.addr = "tcp://" + hostport
		return ep, nil
	}
	return endpoint{}, bad
}
