package main

import (
	"strconv"
	"strings"
)

// mount is one entry of the mount table.
type mount struct {
	Device, Mountpoint, FSType string
}

// parseMounts reads the Linux /proc/self/mounts format. Spaces, tabs,
// newlines and backslashes in the fields are octal escapes (\040 etc.).
func parseMounts(text string) []mount {
	var ms []mount
	for line := range strings.SplitSeq(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		ms = append(ms, mount{Device: unescape(f[0]), Mountpoint: unescape(f[1]), FSType: unescape(f[2])})
	}
	return ms
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// mountOf returns the entry with the longest mount point that holds path.
// On equal mount points the later entry wins: it is mounted over the
// earlier one. If same is not nil, only entries whose mount point is on the
// same filesystem as path are considered: a mount point can be hidden by a
// later mount over one of its parents.
func mountOf(ms []mount, path string, same func(mountpoint string) bool) (mount, bool) {
	var best mount
	found := false
	for _, m := range ms {
		if !contains(m.Mountpoint, path) || (same != nil && !same(m.Mountpoint)) {
			continue
		}
		if !found || len(m.Mountpoint) >= len(best.Mountpoint) {
			best, found = m, true
		}
	}
	return best, found
}

// contains reports whether path is dir or lies below it.
func contains(dir, path string) bool {
	if dir == "/" || dir == path {
		return strings.HasPrefix(path, "/")
	}
	return strings.HasPrefix(path, dir+"/")
}

// mountOfDevice returns the first entry of the device, comparing both sides
// after resolve (which follows symlinks such as /dev/mapper/* or
// /dev/disk/by-uuid/*).
func mountOfDevice(ms []mount, device string, resolve func(string) string) (mount, bool) {
	want := resolve(device)
	for _, m := range ms {
		if m.Device == device || resolve(m.Device) == want {
			return m, true
		}
	}
	return mount{}, false
}
