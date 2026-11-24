package main

import "syscall"

// mntNoWait is MNT_NOWAIT from <sys/mount.h>: return cached statistics.
const mntNoWait = 2

func statfs(path string) (usage, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return usage{}, err
	}
	bs := uint64(st.Bsize)
	return usage{
		Total:     st.Blocks * bs,
		Free:      st.Bfree * bs,
		Avail:     st.Bavail * bs,
		Files:     st.Files,
		FilesFree: st.Ffree,
	}, nil
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
		ms = append(ms, mount{
			Device:     cstring(st.Mntfromname[:]),
			Mountpoint: cstring(st.Mntonname[:]),
			FSType:     cstring(st.Fstypename[:]),
		})
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
