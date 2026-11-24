// Command onecheck-systemd is the onecheck module that reports the state of
// a systemd unit.
package main

import (
	"encoding/json"

	"github.com/alekzonder/onecheck/internal/version"
	"github.com/alekzonder/onecheck/pkg/module"
)

const paramsSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["unit"],
  "properties": {
    "unit": {"type": "string", "minLength": 1, "description": "unit name, e.g. nginx.service or backup.timer"},
    "user": {"type": "boolean", "description": "check a unit of the user's service manager (systemctl --user)"},
    "properties": {"type": "array", "items": {"type": "string", "minLength": 1}, "description": "extra systemctl show properties returned in the properties field"}
  }
}`

var mod = module.Module{
	Description: module.Description{
		Name:         "systemd",
		Version:      version.Version,
		Description:  "Report the state of a systemd unit: active and enabled state, restarts, timer runs",
		ParamsSchema: json.RawMessage(paramsSchema),
		Fields: map[string]module.FieldInfo{
			"load_state":           {Type: module.TypeString, Description: "loaded, not-found, masked, bad-setting, error"},
			"active_state":         {Type: module.TypeString, Description: "active, inactive, failed, activating, deactivating, reloading"},
			"sub_state":            {Type: module.TypeString, Description: "type-specific state: running, exited, dead, waiting, listening, ..."},
			"unit_file_state":      {Type: module.TypeString, Description: "enabled, disabled, static, masked, indirect, ...; empty if systemd reports none"},
			"result":               {Type: module.TypeString, Description: "result of the unit's last run: success, exit-code, signal, timeout, ..."},
			"description":          {Type: module.TypeString, Description: "unit description"},
			"state_age":            {Type: module.TypeDuration, Description: "seconds since the last change of the active or sub state"},
			"restarts":             {Type: module.TypeNumber, Description: "service: automatic restarts since it was started"},
			"main_pid":             {Type: module.TypeNumber, Description: "service: main process id, 0 if not running"},
			"exit_status":          {Type: module.TypeNumber, Description: "service: exit status of the last main process"},
			"service":              {Type: module.TypeString, Description: "timer: the unit it activates"},
			"last_trigger_age":     {Type: module.TypeDuration, Description: "timer: seconds since it last fired; absent if it has not fired"},
			"idle_age":             {Type: module.TypeDuration, Description: "timer: seconds since it last fired or, if it has not fired since, since it was activated"},
			"next_trigger_in":      {Type: module.TypeDuration, Description: "timer: seconds until it fires next; absent if nothing is scheduled"},
			"service_result":       {Type: module.TypeString, Description: "timer: result of the last run of the activated unit"},
			"service_exit_status":  {Type: module.TypeNumber, Description: "timer: exit status of the activated service's last main process"},
			"service_active_state": {Type: module.TypeString, Description: "timer: active state of the activated unit"},
			"properties":           {Type: module.TypeObject, Description: "values of params.properties as printed by systemctl show"},
		},
	},
	Collect: collect,
}

func main() { module.Main(mod) }
