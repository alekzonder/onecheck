package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// maxBody limits a Docker API response read into memory.
const maxBody = 64 << 20

// client talks to the Docker Engine API. Paths are unversioned: the daemon
// answers with its own API version.
type client struct {
	ep   endpoint
	base string
	http *http.Client
}

func newClient(ep endpoint) *client {
	scheme, rest, _ := strings.Cut(ep.addr, "://")
	tr := &http.Transport{DisableKeepAlives: true}
	base := "http://" + rest
	if scheme == "unix" {
		base = "http://docker"
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", rest)
		}
	}
	return &client{ep: ep, base: base, http: &http.Client{Transport: tr}}
}

// get decodes the JSON answer of a GET request into out. It returns the
// HTTP status; a non-2xx status is an error carrying Docker's message.
func (c *client) get(ctx context.Context, path string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", c.ep, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, c.connError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("%s: read response: %w", c.ep, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var msg struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &msg) != nil || msg.Message == "" {
			msg.Message = strings.TrimSpace(string(body))
		}
		return resp.StatusCode, fmt.Errorf("%s: GET %s: %s: %s", c.ep, path, resp.Status, msg.Message)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return resp.StatusCode, fmt.Errorf("%s: GET %s: invalid JSON: %w", c.ep, path, err)
	}
	return resp.StatusCode, nil
}

func (c *client) connError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err // drop the internal "Get http://docker/..." wrapper
	}
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("%s: %w; add the user onecheck runs as to the docker group or run it as root", c.ep, err)
	}
	return fmt.Errorf("%s: %w", c.ep, err)
}
