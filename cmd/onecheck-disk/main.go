// Command onecheck-disk is the onecheck module that reports space and inode
// usage of a filesystem, or of every filesystem of the host.
package main

import (
	"encoding/json"

	"github.com/alekzonder/onecheck/internal/version"
	"github.com/alekzonder/onecheck/pkg/module"
)

const paramsSchema = `{
  "type": "object",
  "additionalProperties": false,
  "if": {"required": ["path"]},
  "then": {"properties": {"exclude_fstype": false, "exclude_mount": false}},
  "properties": {
    "path": {"type": "string", "minLength": 1, "description": "mount point, any path inside a filesystem, or a device under /dev/; all filesystems of the host are listed when omitted"},
    "exclude_fstype": {"type": "array", "items": {"type": "string", "minLength": 1}, "description": "without path: shell patterns of filesystem types to leave out"},
    "exclude_mount": {"type": "array", "items": {"type": "string", "minLength": 1}, "description": "without path: shell patterns of mount points to leave out"}
  }
}`

var mod = module.Module{
	Description: module.Description{
		Name:         "disk",
		Version:      version.Version,
		Description:  "Report free space and inodes of the filesystem holding a path or device, or of every filesystem of the host",
		ParamsSchema: json.RawMessage(paramsSchema),
		Fields: map[string]module.FieldInfo{
			"total":               {Type: module.TypeBytes, Description: "size of the filesystem"},
			"free":                {Type: module.TypeBytes, Description: "space available to unprivileged users (df Avail)"},
			"used":                {Type: module.TypeBytes, Description: "used space: total minus all free blocks"},
			"used_percent":        {Type: module.TypeNumber, Description: "used / (used + free) * 100, like df Use%"},
			"inodes_total":        {Type: module.TypeNumber, Description: "inodes of the filesystem; absent if it has no fixed inode count"},
			"inodes_free":         {Type: module.TypeNumber, Description: "free inodes"},
			"inodes_used":         {Type: module.TypeNumber, Description: "used inodes"},
			"inodes_used_percent": {Type: module.TypeNumber, Description: "inodes_used / inodes_total * 100"},
			"mountpoint":          {Type: module.TypeString, Description: "where the filesystem is mounted"},
			"device":              {Type: module.TypeString, Description: "mounted device as in the mount table"},
			"fstype":              {Type: module.TypeString, Description: "filesystem type, e.g. ext4, xfs, apfs"},
			"filesystems": {
				Type:        module.TypeArray,
				Description: "without params.path: the real filesystems of the host, one per device, sorted by mount point; each has the fields above",
				Elements:    map[string]module.FieldType{"total": module.TypeBytes, "free": module.TypeBytes, "used": module.TypeBytes},
			},
			"count": {Type: module.TypeNumber, Description: "without params.path: number of filesystems"},
		},
	},
	Collect: collect,
}

func main() { module.Main(mod) }
