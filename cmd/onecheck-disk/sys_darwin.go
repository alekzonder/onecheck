package main

import "syscall"

// mntNoWait is MNT_NOWAIT from <sys/mount.h>: return cached statistics.
const mntNoWait = 2

// statfs returns the usage of the filesystem holding path and its mount
// entry, as the kernel reports it. Firmlinks (/Users, /private, ...) lead to
// the Data volume, which a path-prefix lookup in the mount table would miss.
func statfs(path string) (usage, *mount, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return usage{}, nil, err
	}
	bs := uint64(st.Bsize)
	u := usage{
		Total:     st.Blocks * bs,
		Free:      st.Bfree * bs,
		Avail:     st.Bavail * bs,
		Files:     st.Files,
		FilesFree: st.Ffree,
	}
	m := mountOfStatfs(&st)
	return u, &m, nil
}

func mountTable() ([]mount, error) {
	n, err := syscall.Getfsstat(nil, mntNoWait)
	if err != nil {
		return nil, err
	}
	buf := make([]syscall.Statfs_t, n)
	n, err = syscall.Getfsstat(buf, mntNoWait)
	if err != nil {
		return nil, err
	}
	ms := make([]mount, 0, n)
	for _, st := range buf[:n] {
		ms = append(ms, mountOfStatfs(&st))
	}
	return ms, nil
}

func cstring(b []int8) string {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		out = append(out, byte(c))
	}
	return string(out)
}

func mountOfStatfs(st *syscall.Statfs_t) mount {
	return mount{
		Device:     cstring(st.Mntfromname[:]),
		Mountpoint: cstring(st.Mntonname[:]),
		FSType:     cstring(st.Fstypename[:]),
	}
}
