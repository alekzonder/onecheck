// Command onecheck-command is the onecheck module that runs a command.
package main

import (
	"encoding/json"

	"github.com/alekzonder/onecheck/internal/version"
	"github.com/alekzonder/onecheck/pkg/module"
)

const paramsSchema = `{
  "type": "object",
  "additionalProperties": false,
  "oneOf": [{"required": ["run"]}, {"required": ["argv"]}],
  "properties": {
    "run": {"type": "string", "minLength": 1, "description": "command line executed with sh -c"},
    "argv": {"type": "array", "minItems": 1, "items": {"type": "string"}, "description": "program and arguments, executed without a shell"},
    "env": {"type": "object", "additionalProperties": {"type": "string"}, "description": "extra environment variables"},
    "cwd": {"type": "string", "description": "working directory"},
    "stdin": {"type": "string", "description": "text passed to the command on stdin"}
  }
}`

var mod = module.Module{
	Description: module.Description{
		Name:         "command",
		Version:      version.Version,
		Description:  "Run a command and return its exit code, stdout and stderr",
		ParamsSchema: json.RawMessage(paramsSchema),
		Fields: map[string]module.FieldInfo{
			"exit_code": {Type: module.TypeNumber, Description: "exit code of the command (-1 if it was killed by a signal)"},
			"stdout":    {Type: module.TypeString, Description: "standard output"},
			"stderr":    {Type: module.TypeString, Description: "standard error"},
			"duration":  {Type: module.TypeDuration, Description: "seconds the command ran"},
		},
	},
	Collect: collect,
}

func main() { module.Main(mod) }
