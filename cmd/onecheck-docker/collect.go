package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/alekzonder/onecheck/internal/glob"
	"github.com/alekzonder/onecheck/pkg/module"
)

// The environment is reached through these variables so tests can replace it.
var (
	resolve = func(param string) (endpoint, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "" // no HOME: skip ~/.docker and the macOS sockets
		}
		return resolveHost(param, os.Getenv, home, runtime.GOOS, fileExists)
	}
	now = time.Now
)

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

type params struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Host  string `json:"host"`
}

// listItem is an element of GET /containers/json.
type listItem struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Labels map[string]string `json:"Labels"`
}

// name is the container's own name; with legacy links Names also holds
// aliases such as "/app/db", which have a second slash.
func (l listItem) name() string {
	for _, n := range l.Names {
		if n = strings.TrimPrefix(n, "/"); !strings.Contains(n, "/") {
			return n
		}
	}
	return ""
}

// inspect is the part of GET /containers/{id}/json the module reports.
type inspect struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	RestartCount int64  `json:"RestartCount"`
	State        struct {
		Status    string    `json:"Status"`
		OOMKilled bool      `json:"OOMKilled"`
		ExitCode  int64     `json:"ExitCode"`
		StartedAt time.Time `json:"StartedAt"`
		Health    *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	Config struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
}

// filter selects containers by the name and label params.
type filter struct {
	name       *regexp.Regexp
	labelKey   string
	labelValue string
	hasValue   bool
}

func newFilter(p params) (filter, error) {
	var f filter
	if p.Name != "" {
		re, err := glob.Compile(p.Name)
		if err != nil {
			return f, fmt.Errorf("invalid params: name: %w", err)
		}
		f.name = re
	}
	f.labelKey, f.labelValue, f.hasValue = strings.Cut(p.Label, "=")
	return f, nil
}

func (f filter) match(l listItem) bool {
	if f.name != nil && !f.name.MatchString(l.name()) {
		return false
	}
	if f.labelKey == "" {
		return true
	}
	v, ok := l.Labels[f.labelKey]
	return ok && (!f.hasValue || v == f.labelValue)
}

func collect(ctx context.Context, req module.Request) (map[string]any, error) {
	var p params
	if err := module.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	f, err := newFilter(p)
	if err != nil {
		return nil, err
	}
	ep, err := resolve(p.Host)
	if err != nil {
		return nil, err
	}
	c := newClient(ep)
	var list []listItem
	if _, err := c.get(ctx, "/containers/json?all=1", &list); err != nil {
		return nil, err
	}
	var matched []listItem
	for _, l := range list {
		if f.match(l) {
			matched = append(matched, l)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].name() < matched[j].name() })
	if !req.WantsOr("containers", true) {
		return map[string]any{"count": int64(len(matched))}, nil
	}
	out := make([]map[string]any, 0, len(matched))
	for _, l := range matched {
		var in inspect
		status, err := c.get(ctx, "/containers/"+url.PathEscape(l.ID)+"/json", &in)
		if status == http.StatusNotFound {
			continue // removed since it was listed
		}
		if err != nil {
			return nil, err
		}
		out = append(out, containerFields(in, now()))
	}
	return map[string]any{"containers": out, "count": int64(len(out))}, nil
}

// containerFields builds one element of the containers field.
func containerFields(in inspect, at time.Time) map[string]any {
	health := "none"
	if in.State.Health != nil && in.State.Health.Status != "" {
		health = in.State.Health.Status
	}
	var uptime int64
	if (in.State.Status == "running" || in.State.Status == "restarting") && !in.State.StartedAt.IsZero() {
		uptime = max(0, int64(at.Sub(in.State.StartedAt).Seconds()))
	}
	id := in.ID
	if len(id) > 12 {
		id = id[:12]
	}
	labels := in.Config.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return map[string]any{
		"name":           strings.TrimPrefix(in.Name, "/"),
		"id":             id,
		"image":          in.Config.Image,
		"state":          in.State.Status,
		"health":         health,
		"restart_count":  in.RestartCount,
		"exit_code":      in.State.ExitCode,
		"oom_killed":     in.State.OOMKilled,
		"uptime_seconds": uptime,
		"labels":         labels,
	}
}
