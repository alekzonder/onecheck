package main

import (
	"strconv"
	"strings"
	"time"
)

// Properties read from systemctl show. Properties a unit type does not have
// are simply not printed, so one call can ask for all of them.
var (
	commonProps    = []string{"Id", "Description", "LoadState", "ActiveState", "SubState", "UnitFileState", "Result", "StateChangeTimestamp"}
	serviceProps   = []string{"NRestarts", "MainPID", "ExecMainStatus"}
	timerProps     = []string{"Unit", "LastTriggerUSec", "NextElapseUSecRealtime", "ActiveEnterTimestamp"}
	activatedProps = []string{"Result", "ExecMainStatus", "ActiveState"}
)

// unitType is the suffix of a unit name: service, timer, socket, ...
func unitType(id string) string {
	if i := strings.LastIndexByte(id, '.'); i >= 0 {
		return id[i+1:]
	}
	return ""
}

// unitFields turns the properties of a unit into module fields. A property
// that systemd did not print leaves its field out.
func unitFields(p map[string]string, now time.Time) map[string]any {
	out := map[string]any{}
	copyString(out, p, map[string]string{
		"load_state": "LoadState", "active_state": "ActiveState", "sub_state": "SubState",
		"unit_file_state": "UnitFileState", "result": "Result", "description": "Description",
	})
	if t, ok := parseTime(p["StateChangeTimestamp"]); ok {
		out["state_age"] = now.Sub(t).Seconds()
	}
	switch unitType(p["Id"]) {
	case "service":
		copyInt(out, p, map[string]string{"restarts": "NRestarts", "main_pid": "MainPID", "exit_status": "ExecMainStatus"})
	case "timer":
		timerFields(out, p, now)
	}
	return out
}

func timerFields(out map[string]any, p map[string]string, now time.Time) {
	copyString(out, p, map[string]string{"service": "Unit"})
	last, fired := parseTime(p["LastTriggerUSec"])
	if fired {
		out["last_trigger_age"] = now.Sub(last).Seconds()
	}
	// idle_age counts from the activation when the timer has not fired
	// since then: a timer without Persistent=true forgets its last trigger
	// on reboot.
	since, ok := parseTime(p["ActiveEnterTimestamp"])
	if fired && (!ok || last.After(since)) {
		since, ok = last, true
	}
	if ok {
		out["idle_age"] = now.Sub(since).Seconds()
	}
	if next, ok := parseTime(p["NextElapseUSecRealtime"]); ok {
		out["next_trigger_in"] = max(next.Sub(now).Seconds(), 0)
	}
}

// activatedFields reports the last run of the unit a timer activates.
func activatedFields(p map[string]string) map[string]any {
	out := map[string]any{}
	copyString(out, p, map[string]string{"service_result": "Result", "service_active_state": "ActiveState"})
	copyInt(out, p, map[string]string{"service_exit_status": "ExecMainStatus"})
	return out
}

func copyString(out map[string]any, p map[string]string, names map[string]string) {
	for field, prop := range names {
		if v, ok := p[prop]; ok {
			out[field] = v
		}
	}
}

func copyInt(out map[string]any, p map[string]string, names map[string]string) {
	for field, prop := range names {
		if n, err := strconv.ParseInt(p[prop], 10, 64); err == nil {
			out[field] = n
		}
	}
}
