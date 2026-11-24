// Command onecheck-systemd is the onecheck module that reports the state of
// a systemd unit, or the units in a given state.
package main

import (
	"encoding/json"

	"github.com/alekzonder/onecheck/internal/version"
	"github.com/alekzonder/onecheck/pkg/module"
)

const paramsSchema = `{
  "type": "object",
  "additionalProperties": false,
  "oneOf": [{"required": ["unit"]}, {"required": ["state"]}],
  "properties": {
    "unit": {"type": "string", "minLength": 1, "description": "unit name, e.g. nginx.service or backup.timer; no patterns"},
    "state": {"enum": ["failed", "active", "inactive", "activating", "deactivating", "reloading", "maintenance", "refreshing", "loaded", "not-found", "bad-setting", "error", "masked"], "description": "instead of unit: list the units in this load or active state, e.g. failed"},
    "exclude": {"type": "array", "items": {"type": "string", "minLength": 1}, "description": "with state: shell patterns of unit names to leave out"},
    "user": {"type": "boolean", "description": "check a unit of the user's service manager (systemctl --user)"},
    "properties": {"type": "array", "items": {"type": "string", "minLength": 1}, "description": "extra systemctl show properties returned in the properties field"}
  }
}`

var mod = module.Module{
	Description: module.Description{
		Name:         "systemd",
		Version:      version.Version,
		Description:  "Report the state of a systemd unit (active and enabled state, restarts, timer runs) or list the units in a state, e.g. failed",
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
			"idle_age":             {Type: module.TypeDuration, Description: "timer: seconds since it last fired or, if systemd has no last trigger (e.g. after a reboot without Persistent=true), since it was activated"},
			"next_trigger_in":      {Type: module.TypeDuration, Description: "timer: seconds until the next calendar (OnCalendar) run; absent if none is scheduled"},
			"service_result":       {Type: module.TypeString, Description: "timer: result of the last run of the activated unit"},
			"service_exit_status":  {Type: module.TypeNumber, Description: "timer: exit status of the activated service's last main process"},
			"service_active_state": {Type: module.TypeString, Description: "timer: active state of the activated unit"},
			"units":                {Type: module.TypeArray, Description: "with params.state: the units in that state sorted by name; each has name, load_state, active_state, sub_state, description"},
			"names":                {Type: module.TypeArray, Description: "with params.state: the names of those units"},
			"count":                {Type: module.TypeNumber, Description: "with params.state: number of those units"},
			"properties":           {Type: module.TypeObject, Description: "values of params.properties: integers as numbers, everything else as printed by systemctl show"},
		},
	},
	Collect: collect,
}

func main() { module.Main(mod) }
