package main

import (
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
)

func wantFields(t *testing.T, got, want map[string]any) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[k], v)
		}
	}
}

func wantAbsent(t *testing.T, got map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if v, ok := got[k]; ok {
			t.Errorf("%s = %#v, want absent", k, v)
		}
	}
}

var busySample = cpu.TimesStat{User: 30, Nice: 10, System: 10, Irq: 5, Softirq: 5, Idle: 20, Iowait: 10, Steal: 10, Guest: 5}

func TestUsageFieldsLinux(t *testing.T) {
	f := usageFields(cpu.TimesStat{}, busySample, true)
	wantFields(t, f, map[string]any{
		"usage_percent": 70.0, "user_percent": 40.0, "system_percent": 20.0,
		"iowait_percent": 10.0, "steal_percent": 10.0,
	})
}

func TestUsageFieldsDarwin(t *testing.T) {
	f := usageFields(cpu.TimesStat{}, busySample, false)
	wantFields(t, f, map[string]any{"usage_percent": 71.43})
	wantAbsent(t, f, "iowait_percent", "steal_percent")
}

func TestUsageFieldsNoDelta(t *testing.T) {
	f := usageFields(busySample, busySample, true)
	for _, k := range percentFields {
		if f[k] != 0.0 {
			t.Errorf("%s = %#v, want 0", k, f[k])
		}
	}
}

func TestUsageFieldsCounterReset(t *testing.T) {
	f := usageFields(busySample, cpu.TimesStat{}, true)
	for _, k := range percentFields {
		if f[k] != 0.0 {
			t.Errorf("%s = %#v, want 0", k, f[k])
		}
	}
}

func TestLoadFields(t *testing.T) {
	f := loadFields(load.AvgStat{Load1: 1.234, Load5: 2.5, Load15: 3}, 4)
	wantFields(t, f, map[string]any{
		"cores": int64(4), "load1": 1.23, "load5": 2.5, "load15": 3.0,
		"load1_per_core": 0.31, "load5_per_core": 0.63, "load15_per_core": 0.75,
	})
}

func TestLoadFieldsNoCores(t *testing.T) {
	f := loadFields(load.AvgStat{Load1: 1}, 0)
	wantFields(t, f, map[string]any{"load1": 1.0})
	wantAbsent(t, f, "cores", "load1_per_core", "load5_per_core", "load15_per_core")
}

func TestParseWindow(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"": time.Second, "500ms": 500 * time.Millisecond, "2": 2 * time.Second,
		"5m": 5 * time.Minute, "100ms": 100 * time.Millisecond, "1.5s": 1500 * time.Millisecond,
	} {
		got, err := parseWindow(in)
		if err != nil || got != want {
			t.Errorf("parseWindow(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"50ms", "5m1s", "0", "abc", "-1s", "10m"} {
		_, err := parseWindow(in)
		if err == nil || !strings.Contains(err.Error(), in) {
			t.Errorf("parseWindow(%q) err = %v, want an error naming the value", in, err)
		}
	}
}
