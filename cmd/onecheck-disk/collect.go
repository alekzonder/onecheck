package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/alekzonder/onecheck/pkg/module"
)

type params struct {
	// Path is nil when the rule gives none: then all filesystems are listed.
	Path          *string  `json:"path"`
	ExcludeFSType []string `json:"exclude_fstype"`
	ExcludeMount  []string `json:"exclude_mount"`
}

func collect(_ context.Context, req module.Request) (map[string]any, error) {
	var p params
	if err := module.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	if p.Path == nil {
		ms, err := mountTable()
		if err != nil {
			return nil, fmt.Errorf("cannot read the mount table: %w", err)
		}
		return listFilesystems(ms, statMount, p)
	}
	if len(p.ExcludeFSType) > 0 || len(p.ExcludeMount) > 0 {
		return nil, errors.New("invalid params: exclude_fstype and exclude_mount work without path, for the list of all filesystems")
	}
	if *p.Path == "" {
		return nil, errors.New("invalid params: path is empty; leave it out to list all filesystems")
	}
	target, entry, err := resolveTarget(*p.Path)
	if err != nil {
		return nil, err
	}
	u, own, err := statfs(target)
	if err != nil {
		return nil, fmt.Errorf("statfs %s: %w", target, err)
	}
	if own != nil {
		entry = own
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
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil, fmt.Errorf("path %s does not exist", path)
	}
	if err != nil {
		return "", nil, err
	}
	ms, err := mountTable()
	if err != nil {
		return real, nil, nil
	}
	if m, ok := mountOf(ms, real, sameFilesystem(real)); ok {
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

// sameFilesystem returns a test whether a mount point is on the same
// filesystem as path, by comparing device numbers. If path cannot be
// stat'ed, every mount point passes.
func sameFilesystem(path string) func(string) bool {
	dev, ok := deviceOf(path)
	if !ok {
		return nil
	}
	return func(mountpoint string) bool {
		other, ok := deviceOf(mountpoint)
		return !ok || other == dev
	}
}

func deviceOf(path string) (uint64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true //nolint:gosec,unconvert // Dev is int32 on darwin, uint64 on linux
}
