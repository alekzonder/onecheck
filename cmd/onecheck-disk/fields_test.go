package main

import "testing"

func TestUsageFieldsLikeDf(t *testing.T) {
	f := usageFields(usage{Total: 100, Free: 30, Avail: 20})
	want := map[string]any{"total": int64(100), "free": int64(20), "used": int64(70), "used_percent": 77.78}
	for k, v := range want {
		if f[k] != v {
			t.Errorf("%s = %v (%T), want %v", k, f[k], f[k], v)
		}
	}
}

func TestUsageFieldsNoInodes(t *testing.T) {
	f := usageFields(usage{Total: 100, Free: 50, Avail: 50})
	for _, k := range []string{"inodes_total", "inodes_free", "inodes_used", "inodes_used_percent"} {
		if _, has := f[k]; has {
			t.Errorf("%s present for a filesystem without inode count", k)
		}
	}
}

func TestUsageFieldsInodes(t *testing.T) {
	f := usageFields(usage{Total: 100, Free: 50, Avail: 50, Files: 1000, FilesFree: 250})
	want := map[string]any{"inodes_total": int64(1000), "inodes_free": int64(250), "inodes_used": int64(750), "inodes_used_percent": 75.0}
	for k, v := range want {
		if f[k] != v {
			t.Errorf("%s = %v (%T), want %v", k, f[k], f[k], v)
		}
	}
}

func TestUsageFieldsEmpty(t *testing.T) {
	f := usageFields(usage{})
	if f["used_percent"] != 0.0 || f["used"] != int64(0) {
		t.Errorf("fields = %v", f)
	}
}

func TestUsageFieldsFreeAboveTotalDoesNotUnderflow(t *testing.T) {
	f := usageFields(usage{Total: 10, Free: 20, Avail: 20, Files: 5, FilesFree: 9})
	if f["used"] != int64(0) || f["inodes_used"] != int64(0) {
		t.Errorf("fields = %v", f)
	}
}
