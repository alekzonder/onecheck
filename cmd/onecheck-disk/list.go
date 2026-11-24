package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/alekzonder/onecheck/internal/glob"
)

// Filesystem types listed although their device is not a path: network and
// pooled filesystems.
var remoteTypes = map[string]bool{
	"zfs": true, "nfs": true, "nfs4": true, "cifs": true, "smb3": true, "smbfs": true, "ceph": true,
	"9p": true, "virtiofs": true, "lustre": true, "fuse.glusterfs": true, "fuse.sshfs": true,
}

// Filesystem types never listed although their device is a path: read-only
// images, which are full by nature, and container layers.
var imageTypes = map[string]bool{
	"squashfs": true, "iso9660": true, "udf": true, "erofs": true, "cramfs": true, "overlay": true,
}

// real reports whether a mount is a filesystem that holds data and can fill
// up, as opposed to proc, tmpfs, cgroups and the like.
func real(m mount) bool {
	if imageTypes[m.FSType] {
		return false
	}
	return strings.HasPrefix(m.Device, "/") || remoteTypes[m.FSType]
}

// statMount is statfs for a mount point of the mount table.
func statMount(mountpoint string) (usage, error) {
	u, _, err := statfs(mountpoint)
	return u, err
}

// listFilesystems reports the real filesystems of the mount table, one per
// device, without those the exclude patterns match. stat is asked only
// about the filesystems that are left.
func listFilesystems(ms []mount, stat func(mountpoint string) (usage, error), p params) (map[string]any, error) {
	skipType, err := compilePatterns("exclude_fstype", p.ExcludeFSType)
	if err != nil {
		return nil, err
	}
	skipMount, err := compilePatterns("exclude_mount", p.ExcludeMount)
	if err != nil {
		return nil, err
	}
	list := []map[string]any{}
	for _, m := range oncePerDevice(ms) {
		if matchesAny(skipType, m.FSType) || matchesAny(skipMount, m.Mountpoint) {
			continue
		}
		u, err := stat(m.Mountpoint)
		if err != nil {
			return nil, fmt.Errorf("statfs %s (%s on %s): %w; fix the mount or leave it out with exclude_mount",
				m.Mountpoint, m.FSType, m.Device, err)
		}
		if u.Total == 0 {
			continue
		}
		fs := usageFields(u)
		fs["mountpoint"], fs["device"], fs["fstype"] = m.Mountpoint, m.Device, m.FSType
		list = append(list, fs)
	}
	return map[string]any{"filesystems": list, "count": len(list)}, nil
}

// oncePerDevice returns the real filesystems sorted by mount point, each
// device once: at its shortest mount point, the first of them in the table.
// Bind mounts and subvolumes of a device are the same space.
func oncePerDevice(ms []mount) []mount {
	best := map[string]mount{}
	for _, m := range ms {
		if !real(m) {
			continue
		}
		if cur, seen := best[m.Device]; !seen || len(m.Mountpoint) < len(cur.Mountpoint) {
			best[m.Device] = m
		}
	}
	out := make([]mount, 0, len(best))
	for _, m := range best {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mountpoint != out[j].Mountpoint {
			return out[i].Mountpoint < out[j].Mountpoint
		}
		return out[i].Device < out[j].Device
	})
	return out
}

func compilePatterns(key string, patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := glob.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid params: %s: %w", key, err)
		}
		out = append(out, re)
	}
	return out, nil
}

func matchesAny(patterns []*regexp.Regexp, s string) bool {
	for _, re := range patterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
