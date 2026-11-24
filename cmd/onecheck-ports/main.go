// Command onecheck-ports is the onecheck module that reports the ports the
// host listens on.
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
    "proto": {"enum": ["tcp", "udp"], "description": "only this protocol; both when omitted"},
    "interface": {"type": "string", "minLength": 1, "description": "only sockets bound to an address of this interface (e.g. eth0) or to all addresses; everything but loopback when omitted"}
  }
}`

var mod = module.Module{
	Description: module.Description{
		Name:         "ports",
		Version:      version.Version,
		Description:  "Report listening TCP and UDP ports",
		ParamsSchema: json.RawMessage(paramsSchema),
		Fields: map[string]module.FieldInfo{
			"listening": {Type: module.TypeArray, Description: "listening sockets sorted by proto, port, address; each has proto, port, address, family, interface, process"},
			"count":     {Type: module.TypeNumber, Description: "number of listening sockets"},
		},
	},
	Collect: collect,
}

func main() { module.Main(mod) }
