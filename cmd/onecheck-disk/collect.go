package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/onecheck/pkg/module"
)

type params struct {
	Path string `json:"path"`
}

func collect(_ context.Context, req module.Request) (map[string]any, error) {
	var p params
	if err := module.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	if p.Path == "" {
		return nil, errors.New("invalid params: path is required")
	}
	target, entry, err := resolveTarget(p.Path)
	if err != nil {
		return nil, err
	}
	u, err := statfs(target)
	if err != nil {
		return nil, fmt.Errorf("statfs %s: %w", target, err)
	}
	out := usageFields(u)
	if entry != nil {
		out["mountpoint"] = entry.Mountpoint
		out["device"] = entry.Device
		out["fstype"] = entry.FSType
	}
	return out, nil
}

// resolveTarget returns the path to call statfs on and, when the mount
// table can be read, the entry of its filesystem.
func resolveTarget(path string) (string, *mount, error) {
	if isDevice(path) {
		ms, err := mountTable()
		if err != nil {
			return "", nil, fmt.Errorf("cannot read the mount table to find device %s: %w", path, err)
		}
		m, ok := mountOfDevice(ms, path, resolveLink)
		if !ok {
			return "", nil, fmt.Errorf("device %s is not mounted", path)
		}
		return m.Mountpoint, &m, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", nil, err
	}
	ms, err := mountTable()
	if err != nil {
		return real, nil, nil
	}
	if m, ok := mountOf(ms, real); ok {
		return real, &m, nil
	}
	return real, nil, nil
}

// isDevice reports whether path names a block or character device under /dev/.
func isDevice(path string) bool {
	if !strings.HasPrefix(path, "/dev/") {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode()&fs.ModeDevice != 0
}

// resolveLink follows symlinks of absolute paths; anything else (tmpfs,
// server:/export) is returned unchanged.
func resolveLink(p string) string {
	if !strings.HasPrefix(p, "/") {
		return p
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}
