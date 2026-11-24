package main

import (
	"fmt"
	"math"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"

	"github.com/alekzonder/onecheck/internal/units"
)

const (
	defaultWindow = time.Second
	minWindow     = 100 * time.Millisecond
	maxWindow     = 5 * time.Minute
)

// percentFields are the fields that need two samples a window apart.
var percentFields = []string{"usage_percent", "user_percent", "system_percent", "iowait_percent", "steal_percent"}

// parseWindow reads the window param: an onecheck duration, a bare number
// meaning seconds, empty for the default.
func parseWindow(s string) (time.Duration, error) {
	if s == "" {
		return defaultWindow, nil
	}
	secs, err := units.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid params: window %q: %w", s, err)
	}
	d := time.Duration(math.Round(secs * float64(time.Second)))
	if d < minWindow || d > maxWindow {
		return 0, fmt.Errorf("invalid params: window %q must be from %v to %v", s, minWindow, maxWindow)
	}
	return d, nil
}

// usageFields computes CPU usage between two samples of the summed CPU
// times, in percent of all cores. On Linux guest time is already part of
// user and nice time, so it is left out of the total, as gopsutil does.
func usageFields(a, b cpu.TimesStat, linux bool) map[string]any {
	d := func(x, y float64) float64 { return y - x }
	user := d(a.User, b.User) + d(a.Nice, b.Nice)
	system := d(a.System, b.System) + d(a.Irq, b.Irq) + d(a.Softirq, b.Softirq)
	idle := d(a.Idle, b.Idle)
	iowait := d(a.Iowait, b.Iowait)
	steal := d(a.Steal, b.Steal)
	total := d(sum(a), sum(b))
	if linux {
		total -= d(a.Guest, b.Guest) + d(a.GuestNice, b.GuestNice)
	}
	pct := func(part float64) float64 {
		if total <= 0 {
			return 0
		}
		return round2(math.Min(100, math.Max(0, part/total*100)))
	}
	out := map[string]any{
		"usage_percent":  pct(total - idle - iowait),
		"user_percent":   pct(user),
		"system_percent": pct(system),
	}
	if linux {
		out["iowait_percent"] = pct(iowait)
		out["steal_percent"] = pct(steal)
	}
	return out
}

// loadFields reports the load average and, when the number of cores is
// known, the load per core.
func loadFields(avg load.AvgStat, cores int) map[string]any {
	out := map[string]any{
		"load1":  round2(avg.Load1),
		"load5":  round2(avg.Load5),
		"load15": round2(avg.Load15),
	}
	if cores > 0 {
		c := float64(cores)
		out["cores"] = int64(cores)
		out["load1_per_core"] = round2(avg.Load1 / c)
		out["load5_per_core"] = round2(avg.Load5 / c)
		out["load15_per_core"] = round2(avg.Load15 / c)
	}
	return out
}

// sum adds up every part of the CPU times.
func sum(t cpu.TimesStat) float64 {
	return t.User + t.Nice + t.System + t.Irq + t.Softirq + t.Idle + t.Iowait + t.Steal + t.Guest + t.GuestNice
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
