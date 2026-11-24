package main

import (
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

func wantFields(t *testing.T, f map[string]any, want map[string]any) {
	t.Helper()
	for k, v := range want {
		got, ok := f[k]
		if !ok || got != v {
			t.Errorf("%s = %v (%T), present %v; want %v (%T)", k, got, got, ok, v, v)
		}
	}
}

func wantAbsent(t *testing.T, f map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if v, ok := f[k]; ok {
			t.Errorf("%s = %v, want absent", k, v)
		}
	}
}

func TestUnitFieldsService(t *testing.T) {
	f := unitFields(map[string]string{
		"Id": "nginx.service", "Description": "web", "ActiveState": "active", "SubState": "running",
		"LoadState": "loaded", "UnitFileState": "enabled", "Result": "success",
		"StateChangeTimestamp": "Tue 2026-11-24 11:00:00 UTC",
		"NRestarts":            "2", "MainPID": "123", "ExecMainStatus": "0",
	}, testNow)
	wantFields(t, f, map[string]any{
		"active_state": "active", "sub_state": "running", "load_state": "loaded",
		"unit_file_state": "enabled", "result": "success", "description": "web",
		"state_age": 3600.0, "restarts": int64(2), "main_pid": int64(123), "exit_status": int64(0),
	})
	wantAbsent(t, f, "last_trigger_age", "idle_age", "service")
}

func TestUnitFieldsTimer(t *testing.T) {
	f := unitFields(map[string]string{
		"Id": "backup.timer", "Unit": "backup.service",
		"LastTriggerUSec":        "Tue 2026-11-24 02:00:00 UTC",
		"NextElapseUSecRealtime": "Wed 2026-11-25 02:00:00 UTC",
		"ActiveEnterTimestamp":   "Fri 2026-11-13 14:00:00 UTC",
	}, testNow)
	wantFields(t, f, map[string]any{
		"service": "backup.service", "last_trigger_age": 36000.0,
		"idle_age": 36000.0, "next_trigger_in": 50400.0,
	})
	wantAbsent(t, f, "restarts", "main_pid", "exit_status")
}

func TestUnitFieldsTimerNeverFired(t *testing.T) {
	f := unitFields(map[string]string{
		"Id": "backup.timer", "Unit": "backup.service",
		"LastTriggerUSec": "n/a", "NextElapseUSecRealtime": "",
		"ActiveEnterTimestamp": "Tue 2026-11-24 10:00:00 UTC",
	}, testNow)
	wantFields(t, f, map[string]any{"idle_age": 7200.0})
	wantAbsent(t, f, "last_trigger_age", "next_trigger_in")
}

func TestUnitFieldsNextTriggerInPastClamped(t *testing.T) {
	f := unitFields(map[string]string{
		"Id": "backup.timer", "NextElapseUSecRealtime": "Tue 2026-11-24 11:00:00 UTC",
	}, testNow)
	wantFields(t, f, map[string]any{"next_trigger_in": 0.0})
}

func TestUnitFieldsNotFound(t *testing.T) {
	f := unitFields(map[string]string{
		"Id": "nosuch.service", "LoadState": "not-found", "ActiveState": "inactive",
		"UnitFileState": "", "StateChangeTimestamp": "",
	}, testNow)
	wantFields(t, f, map[string]any{"load_state": "not-found", "active_state": "inactive", "unit_file_state": ""})
	wantAbsent(t, f, "state_age")
}

func TestUnitFieldsMissingBuiltinOmitted(t *testing.T) {
	f := unitFields(map[string]string{"Id": "old.service", "MainPID": "0"}, testNow)
	wantAbsent(t, f, "restarts", "exit_status")
	wantFields(t, f, map[string]any{"main_pid": int64(0)})
}

func TestActivatedFields(t *testing.T) {
	f := activatedFields(map[string]string{"Result": "exit-code", "ExecMainStatus": "2", "ActiveState": "inactive"})
	wantFields(t, f, map[string]any{
		"service_result": "exit-code", "service_exit_status": int64(2), "service_active_state": "inactive",
	})
}

func TestPropertyValue(t *testing.T) {
	cases := map[string]any{"1073152": int64(1073152), "-1": int64(-1), "infinity": "infinity", "yes": "yes", "": "", "1.5": "1.5", "[not set]": "[not set]"}
	for in, want := range cases {
		if got := propertyValue(in); got != want {
			t.Errorf("propertyValue(%q) = %v (%T), want %v (%T)", in, got, got, want, want)
		}
	}
}
