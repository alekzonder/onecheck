// Command onecheck-docker is the onecheck module that reports Docker
// containers.
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
    "name": {"type": "string", "minLength": 1, "description": "container name, exact or a shell pattern such as app-*"},
    "label": {"type": "string", "minLength": 1, "description": "label key, or key=value"},
    "host": {"type": "string", "minLength": 1, "description": "Docker address: unix:///path or tcp://host:port; found automatically when omitted"}
  }
}`

var mod = module.Module{
	Description: module.Description{
		Name:         "docker",
		Version:      version.Version,
		Description:  "Report Docker containers: state, health, restarts, image",
		ParamsSchema: json.RawMessage(paramsSchema),
		Fields: map[string]module.FieldInfo{
			"containers": {
				Type:        module.TypeArray,
				Description: "matching containers sorted by name; each has name, id, image, state, health, restart_count, exit_code, oom_killed, uptime_seconds, labels",
				Elements:    map[string]module.FieldType{"uptime_seconds": module.TypeDuration},
			},
			"count": {Type: module.TypeNumber, Description: "number of matching containers"},
		},
	},
	Collect: collect,
}

func main() { module.Main(mod) }
