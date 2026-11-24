// Command onecheck-cpu is the onecheck module that reports CPU usage over a
// window, the load average and the number of cores.
package main

import (
	"encoding/json"

	"github.com/alekzonder/onecheck/internal/version"
	"github.com/alekzonder/onecheck/pkg/module"
)

const paramsSchema = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "window": {"type": "string", "minLength": 1, "description": "how long CPU usage is measured, e.g. 500ms, 5s; default 1s, from 100ms to 5m"}
  }
}`

var mod = module.Module{
	Description: module.Description{
		Name:         "cpu",
		Version:      version.Version,
		Description:  "Report CPU usage over a window, load average and the number of cores",
		ParamsSchema: json.RawMessage(paramsSchema),
		Fields: map[string]module.FieldInfo{
			"usage_percent":   {Type: module.TypeNumber, Description: "busy time of all cores over the window: everything but idle and iowait"},
			"user_percent":    {Type: module.TypeNumber, Description: "user and nice time over the window"},
			"system_percent":  {Type: module.TypeNumber, Description: "system and interrupt time over the window"},
			"iowait_percent":  {Type: module.TypeNumber, Description: "time idle while waiting for disk over the window; Linux only"},
			"steal_percent":   {Type: module.TypeNumber, Description: "time taken by the hypervisor over the window; Linux only"},
			"cores":           {Type: module.TypeNumber, Description: "logical CPUs"},
			"load1":           {Type: module.TypeNumber, Description: "load average over 1 minute"},
			"load5":           {Type: module.TypeNumber, Description: "load average over 5 minutes"},
			"load15":          {Type: module.TypeNumber, Description: "load average over 15 minutes"},
			"load1_per_core":  {Type: module.TypeNumber, Description: "load1 / cores"},
			"load5_per_core":  {Type: module.TypeNumber, Description: "load5 / cores"},
			"load15_per_core": {Type: module.TypeNumber, Description: "load15 / cores"},
		},
	},
	Collect: collect,
}

func main() { module.Main(mod) }
