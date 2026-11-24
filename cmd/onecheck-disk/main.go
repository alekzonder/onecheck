// Command onecheck-disk is the onecheck module that reports space and inode
// usage of a filesystem.
package main

import (
	"encoding/json"

	"github.com/alekzonder/onecheck/internal/version"
	"github.com/alekzonder/onecheck/pkg/module"
)

const paramsSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["path"],
  "properties": {
    "path": {"type": "string", "minLength": 1, "description": "mount point, any path inside a filesystem, or a device under /dev/"}
  }
}`

var mod = module.Module{
	Description: module.Description{
		Name:         "disk",
		Version:      version.Version,
		Description:  "Report free space and inodes of the filesystem holding a path or device",
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
		},
	},
	Collect: collect,
}

func main() { module.Main(mod) }
