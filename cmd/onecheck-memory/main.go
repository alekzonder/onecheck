// Command onecheck-memory is the onecheck module that reports physical
// memory and swap usage of the host.
package main

import (
	"encoding/json"

	"github.com/alekzonder/onecheck/internal/version"
	"github.com/alekzonder/onecheck/pkg/module"
)

const paramsSchema = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {}
}`

var mod = module.Module{
	Description: module.Description{
		Name:         "memory",
		Version:      version.Version,
		Description:  "Report physical memory and swap usage",
		ParamsSchema: json.RawMessage(paramsSchema),
		Fields: map[string]module.FieldInfo{
			"total":             {Type: module.TypeBytes, Description: "physical memory"},
			"available":         {Type: module.TypeBytes, Description: "memory that can be given to programs without swapping"},
			"used":              {Type: module.TypeBytes, Description: "total minus available; reclaimable page cache is not counted"},
			"used_percent":      {Type: module.TypeNumber, Description: "used / total * 100"},
			"swap_total":        {Type: module.TypeBytes, Description: "swap size; grows and shrinks on macOS"},
			"swap_used":         {Type: module.TypeBytes, Description: "swap in use"},
			"swap_free":         {Type: module.TypeBytes, Description: "free swap"},
			"swap_used_percent": {Type: module.TypeNumber, Description: "swap_used / swap_total * 100; 0 without swap"},
		},
	},
	Collect: collect,
}

func main() { module.Main(mod) }
