package main

import (
	"os"
	"syscall"
)

// statfs returns the usage of the filesystem holding path. On Linux the
// mount entry comes from /proc/self/mounts, so the second value is nil.
func statfs(path string) (usage, *mount, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return usage{}, nil, err
	}
	bs := uint64(st.Frsize)
	if bs == 0 {
		bs = uint64(st.Bsize)
	}
	return usage{
		Total:     st.Blocks * bs,
		Free:      st.Bfree * bs,
		Avail:     st.Bavail * bs,
		Files:     st.Files,
		FilesFree: st.Ffree,
	}, nil, nil
}

func mountTable() ([]mount, error) {
	data, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return nil, err
	}
	return parseMounts(string(data)), nil
}
