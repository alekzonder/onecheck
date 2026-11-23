// Command onecheck-file is the onecheck module that inspects files and
// directories.
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
    "path": {"type": "string", "minLength": 1, "description": "file, directory or symlink to inspect"}
  }
}`

var mod = module.Module{
	Description: module.Description{
		Name:         "file",
		Version:      version.Version,
		Description:  "Inspect a file, directory or symlink: existence, type, size, age, permissions, owner, content",
		ParamsSchema: json.RawMessage(paramsSchema),
		Fields: map[string]module.FieldInfo{
			"exists":  {Type: module.TypeBool, Description: "whether the path exists (a dangling symlink does not)"},
			"type":    {Type: module.TypeString, Description: "file, dir, symlink or other; symlink describes the link itself"},
			"size":    {Type: module.TypeBytes, Description: "size in bytes; for a directory the recursive sum of its files"},
			"age":     {Type: module.TypeDuration, Description: "seconds since the last modification"},
			"mode":    {Type: module.TypeString, Description: `permission bits as an octal string, e.g. "0640"`},
			"owner":   {Type: module.TypeString, Description: "owner user name (numeric id if the name is unknown)"},
			"group":   {Type: module.TypeString, Description: "owner group name (numeric id if the name is unknown)"},
			"content": {Type: module.TypeString, Description: "file content (regular files only)"},
			"count":   {Type: module.TypeNumber, Description: "number of entries in a directory, not recursive"},
			"target":  {Type: module.TypeString, Description: "where a symlink points"},
		},
	},
	Collect: collect,
}

func main() { module.Main(mod) }
