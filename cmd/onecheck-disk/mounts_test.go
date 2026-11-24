package main

import "testing"

const procMounts = `sysfs /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
/dev/sda1 / ext4 rw,relatime 0 0
/dev/sda2 /var xfs rw,relatime 0 0
/dev/sdb1 /mnt/my\040disk ext4 rw 0 0
tmpfs /run tmpfs rw,nosuid 0 0
/dev/dm-0 /data ext4 rw 0 0
`

func TestParseMountsDecodesEscapes(t *testing.T) {
	ms := parseMounts(procMounts)
	if len(ms) != 6 {
		t.Fatalf("got %d entries: %v", len(ms), ms)
	}
	want := mount{Device: "/dev/sdb1", Mountpoint: "/mnt/my disk", FSType: "ext4"}
	if ms[3] != want {
		t.Errorf("entry 3 = %+v, want %+v", ms[3], want)
	}
	if got := parseMounts(`a\011b /x\134y fs rw 0 0`)[0]; got.Device != "a\tb" || got.Mountpoint != `/x\y` {
		t.Errorf("escapes: %+v", got)
	}
}

func TestParseMountsSkipsShortLines(t *testing.T) {
	if ms := parseMounts("\nbroken line\n/dev/sda1 / ext4 rw 0 0\n"); len(ms) != 1 || ms[0].Mountpoint != "/" {
		t.Errorf("got %v", ms)
	}
}

func TestMountOfLongestPrefix(t *testing.T) {
	ms := parseMounts(procMounts)
	cases := map[string]string{
		"/variable":         "/",
		"/var":              "/var",
		"/var/lib/postgres": "/var",
		"/":                 "/",
		"/mnt/my disk/x":    "/mnt/my disk",
		"/mnt/my diskette":  "/",
	}
	for path, want := range cases {
		m, ok := mountOf(ms, path, nil)
		if !ok || m.Mountpoint != want {
			t.Errorf("mountOf(%q) = %q, %v; want %q", path, m.Mountpoint, ok, want)
		}
	}
	if _, ok := mountOf(nil, "/x", nil); ok {
		t.Error("empty table must not match")
	}
}

func TestMountOfLaterEntryWins(t *testing.T) {
	ms := parseMounts("/dev/sda2 /var xfs rw 0 0\ntmpfs /var tmpfs rw 0 0\n")
	if m, _ := mountOf(ms, "/var/x", nil); m.Device != "tmpfs" {
		t.Errorf("got %+v, want the later overmount", m)
	}
}

func TestMountOfDeviceResolvesSymlinks(t *testing.T) {
	ms := parseMounts(procMounts)
	resolve := func(p string) string {
		if p == "/dev/mapper/vg-data" {
			return "/dev/dm-0"
		}
		return p
	}
	m, ok := mountOfDevice(ms, "/dev/mapper/vg-data", resolve)
	if !ok || m.Mountpoint != "/data" {
		t.Errorf("got %+v, %v", m, ok)
	}
	if m, ok := mountOfDevice(ms, "/dev/sda2", resolve); !ok || m.Mountpoint != "/var" {
		t.Errorf("plain device: %+v, %v", m, ok)
	}
	if _, ok := mountOfDevice(ms, "/dev/sdz9", resolve); ok {
		t.Error("unmounted device matched")
	}
}

func TestMountOfSkipsEntriesOfAnotherFilesystem(t *testing.T) {
	// /dev/sdb on /mnt/a/b, then /dev/sdc mounted over /mnt/a hides it:
	// /mnt/a/b now lies on sdc.
	ms := parseMounts("/dev/sda1 / ext4 rw 0 0\n/dev/sdb /mnt/a/b ext4 rw 0 0\n/dev/sdc /mnt/a ext4 rw 0 0\n")
	same := func(mp string) bool { return mp != "/mnt/a/b" }
	if m, ok := mountOf(ms, "/mnt/a/b", same); !ok || m.Device != "/dev/sdc" {
		t.Errorf("got %+v, %v; want /dev/sdc", m, ok)
	}
	if _, ok := mountOf(ms, "/mnt/a/b", func(string) bool { return false }); ok {
		t.Error("matched although no entry is on the same filesystem")
	}
}
