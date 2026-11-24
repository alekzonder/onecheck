package main

import "math"

// usage is what statfs reports, converted to bytes and inode counts.
type usage struct {
	Total     uint64 // size of the filesystem, bytes
	Free      uint64 // all free space, including blocks reserved for root, bytes
	Avail     uint64 // free space available to unprivileged users, bytes
	Files     uint64 // inodes; 0 when the filesystem has no fixed inode count
	FilesFree uint64 // free inodes
}

// usageFields turns usage into module fields, computed the way df does.
func usageFields(u usage) map[string]any {
	used := sub(u.Total, u.Free)
	out := map[string]any{
		"total":        int64(u.Total),
		"free":         int64(u.Avail),
		"used":         int64(used),
		"used_percent": percent(used, used+u.Avail),
	}
	if u.Files > 0 {
		iused := sub(u.Files, u.FilesFree)
		out["inodes_total"] = int64(u.Files)
		out["inodes_free"] = int64(u.FilesFree)
		out["inodes_used"] = int64(iused)
		out["inodes_used_percent"] = percent(iused, u.Files)
	}
	return out
}

func sub(a, b uint64) uint64 {
	if b > a {
		return 0
	}
	return a - b
}

// percent is part/whole*100 rounded to two decimals; 0 for an empty whole.
func percent(part, whole uint64) float64 {
	if whole == 0 {
		return 0
	}
	return math.Round(float64(part)/float64(whole)*10000) / 100
}
