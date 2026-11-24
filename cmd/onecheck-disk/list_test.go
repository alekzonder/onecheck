package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/alekzonder/onecheck/pkg/module"
)

var hostMounts = []mount{
	{"sysfs", "/sys", "sysfs"},
	{"proc", "/proc", "proc"},
	{"udev", "/dev", "devtmpfs"},
	{"tmpfs", "/run", "tmpfs"},
	{"/dev/vda1", "/", "ext4"},
	{"/dev/vda1", "/var/lib/docker/volumes/x", "ext4"},
	{"/dev/mapper/vg-data", "/srv/data/sub", "xfs"},
	{"/dev/mapper/vg-data", "/srv/data", "xfs"},
	{"/dev/mapper/vg-data", "/mnt/data", "xfs"},
	{"overlay", "/var/lib/docker/overlay2/abc/merged", "overlay"},
	{"/dev/loop3", "/snap/core22/1", "squashfs"},
	{"tank/home", "/home", "zfs"},
	{"tank/backup", "/backup", "zfs"},
	{"nas:/export", "/mnt/nas", "nfs4"},
	{"/dev/sr0", "/media/cdrom", "iso9660"},
	{"/dev/sdz1", "/mnt/empty", "vfat"},
	{"portal", "/run/user/1000/doc", "fuse.portal"},
}

// fakeStat reports 100 GiB filesystems that are 25 % full; /mnt/empty has no
// size and /home no fixed inode count. It records what was queried.
func fakeStat(asked *[]string) func(string) (usage, error) {
	return func(mountpoint string) (usage, error) {
		*asked = append(*asked, mountpoint)
		switch mountpoint {
		case "/mnt/empty":
			return usage{}, nil
		case "/mnt/nas":
			return usage{}, errors.New("stale file handle")
		case "/home":
			return usage{Total: 100 << 30, Free: 75 << 30, Avail: 75 << 30}, nil
		}
		return usage{Total: 100 << 30, Free: 75 << 30, Avail: 70 << 30, Files: 1000, FilesFree: 900}, nil
	}
}

func mountpoints(t *testing.T, fields map[string]any) []string {
	t.Helper()
	var out []string
	for _, fs := range fields["filesystems"].([]map[string]any) {
		out = append(out, fs["mountpoint"].(string))
	}
	if fields["count"] != len(out) {
		t.Errorf("count = %v for %d filesystems", fields["count"], len(out))
	}
	return out
}

func TestListRealFilesystemsOncePerDevice(t *testing.T) {
	var asked []string
	f, err := listFilesystems(hostMounts, fakeStat(&asked), params{ExcludeMount: []string{"/mnt/nas"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/", "/backup", "/home", "/srv/data"}
	if got := mountpoints(t, f); !reflect.DeepEqual(got, want) {
		t.Errorf("mount points = %v, want %v", got, want)
	}
	for _, mp := range asked {
		if mp == "/mnt/nas" || mp == "/proc" || mp == "/srv/data/sub" || mp == "/mnt/data" || strings.HasPrefix(mp, "/snap") {
			t.Errorf("%s must not be queried", mp)
		}
	}
}

func TestListElementFields(t *testing.T) {
	var asked []string
	f, err := listFilesystems(hostMounts, fakeStat(&asked), params{ExcludeFSType: []string{"nfs*"}})
	if err != nil {
		t.Fatal(err)
	}
	fss := f["filesystems"].([]map[string]any)
	root := fss[0]
	want := map[string]any{
		"mountpoint": "/", "device": "/dev/vda1", "fstype": "ext4",
		"total": int64(100 << 30), "free": int64(70 << 30), "used": int64(25 << 30), "used_percent": 26.32,
		"inodes_total": int64(1000), "inodes_free": int64(900), "inodes_used": int64(100), "inodes_used_percent": 10.0,
	}
	if !reflect.DeepEqual(root, want) {
		t.Errorf("root = %v\nwant   %v", root, want)
	}
	home := fss[2]
	if home["mountpoint"] != "/home" || home["fstype"] != "zfs" {
		t.Fatalf("third element = %v", home)
	}
	if _, has := home["inodes_total"]; has {
		t.Errorf("a filesystem without a fixed inode count has no inode keys: %v", home)
	}
}

func TestListExcludes(t *testing.T) {
	var asked []string
	f, err := listFilesystems(hostMounts, fakeStat(&asked), params{
		ExcludeFSType: []string{"zfs", "nfs?"}, ExcludeMount: []string{"/mnt/*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := mountpoints(t, f); !reflect.DeepEqual(got, []string{"/", "/srv/data"}) {
		t.Errorf("mount points = %v", got)
	}
	for _, p := range []params{{ExcludeFSType: []string{"[z-a]"}}, {ExcludeMount: []string{"[z-a]"}}} {
		if _, err := listFilesystems(hostMounts, fakeStat(&asked), p); err == nil || !strings.Contains(err.Error(), "invalid params") {
			t.Errorf("an invalid pattern must be an error, got %v", err)
		}
	}
}

func TestListFilesystemThatCannotBeQueriedIsAnError(t *testing.T) {
	var asked []string
	_, err := listFilesystems(hostMounts, fakeStat(&asked), params{})
	if err == nil || !strings.Contains(err.Error(), "/mnt/nas") || !strings.Contains(err.Error(), "stale file handle") ||
		!strings.Contains(err.Error(), "exclude_mount") {
		t.Errorf("err = %v", err)
	}
}

func TestListNothingIsAnEmptyList(t *testing.T) {
	var asked []string
	f, err := listFilesystems(hostMounts[:4], fakeStat(&asked), params{})
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := json.Marshal(f); string(data) != `{"count":0,"filesystems":[]}` {
		t.Errorf("fields = %s", data)
	}
}

func TestExcludeNeedsNoPath(t *testing.T) {
	for _, p := range []map[string]any{
		{"path": "/", "exclude_fstype": []string{"zfs"}},
		{"path": "/", "exclude_mount": []string{"/mnt/*"}},
	} {
		raw, _ := json.Marshal(p)
		if _, err := collect(context.Background(), module.Request{Params: raw}); err == nil || !strings.Contains(err.Error(), "invalid params") {
			t.Errorf("%v: err = %v", p, err)
		}
	}
}

func TestCollectWithoutPathListsTheHost(t *testing.T) {
	f, err := collect(context.Background(), module.Request{Params: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	fss, ok := f["filesystems"].([]map[string]any)
	if !ok || f["count"] != len(fss) {
		t.Fatalf("fields = %v", f)
	}
	if _, has := f["total"]; has {
		t.Error("the list has no fields of a single filesystem")
	}
	seen := map[string]bool{}
	for _, fs := range fss {
		dev := fs["device"].(string)
		if seen[dev] {
			t.Errorf("device %s is listed twice", dev)
		}
		seen[dev] = true
		if total, _ := fs["total"].(int64); total <= 0 {
			t.Errorf("%v has no size", fs)
		}
	}
}

func TestDescribeTypesElementSizes(t *testing.T) {
	fi := mod.Description.Fields["filesystems"]
	if fi.Type != module.TypeArray || fi.Elements["free"] != module.TypeBytes || fi.Elements["total"] != module.TypeBytes ||
		fi.Elements["used"] != module.TypeBytes {
		t.Errorf("filesystems = %+v", fi)
	}
	if mod.Description.Fields["count"].Type != module.TypeNumber {
		t.Error("count must be a number")
	}
}

func TestListOrderIsStableForEqualMountPoints(t *testing.T) {
	ms := []mount{{"/dev/sdb1", "/data", "ext4"}, {"/dev/sda1", "/data", "ext4"}, {"/dev/sdc1", "/data", "ext4"}}
	for range 50 {
		var asked []string
		f, err := listFilesystems(ms, fakeStat(&asked), params{})
		if err != nil {
			t.Fatal(err)
		}
		var devs []string
		for _, fs := range f["filesystems"].([]map[string]any) {
			devs = append(devs, fs["device"].(string))
		}
		if !reflect.DeepEqual(devs, []string{"/dev/sda1", "/dev/sdb1", "/dev/sdc1"}) {
			t.Fatalf("devices = %v", devs)
		}
	}
}
