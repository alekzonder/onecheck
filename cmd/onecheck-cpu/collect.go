package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"runtime"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"

	"github.com/alekzonder/onecheck/pkg/module"
)

// The system is read through these variables so tests can replace it.
var (
	cpuTimes = func(ctx context.Context) (cpu.TimesStat, error) {
		ts, err := cpu.TimesWithContext(ctx, false)
		if err != nil {
			return cpu.TimesStat{}, err
		}
		if len(ts) == 0 {
			return cpu.TimesStat{}, errors.New("no CPU times reported")
		}
		return ts[0], nil
	}
	cpuCount = func(ctx context.Context) (int, error) { return cpu.CountsWithContext(ctx, true) }
	loadAvg  = load.AvgWithContext
	sleep    = func(ctx context.Context, d time.Duration) error {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
)

// linux is whether the CPU reports iowait and steal time.
var linux = runtime.GOOS == "linux"

// linuxOnly are percent fields that exist only on Linux.
var linuxOnly = map[string]bool{"iowait_percent": true, "steal_percent": true}

type params struct {
	// Window is a duration string or a bare number of seconds.
	Window any `json:"window"`
}

func collect(ctx context.Context, req module.Request) (map[string]any, error) {
	var p params
	if err := module.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	var s string
	switch v := p.Window.(type) {
	case nil:
	case string:
		s = v
	case float64:
		s = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return nil, fmt.Errorf("invalid params: window must be a duration such as 5s or a number of seconds, got %v", v)
	}
	window, err := parseWindow(s)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if wantsUsage(req) {
		usage, err := measure(ctx, window)
		if err != nil {
			return nil, err
		}
		maps.Copy(out, usage)
	}
	cores, err := cpuCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the number of CPUs: %w", err)
	}
	avg, err := loadAvg(ctx)
	if err != nil {
		return nil, fmt.Errorf("read load average: %w", err)
	}
	maps.Copy(out, loadFields(*avg, cores))
	return out, nil
}

// wantsUsage reports whether a field that needs waiting the window is
// asked for. Waiting is not cheap, so an empty field list skips it.
func wantsUsage(req module.Request) bool {
	for _, f := range percentFields {
		if linuxOnly[f] && !linux {
			continue
		}
		if req.WantsOr(f, false) {
			return true
		}
	}
	return false
}

// measure samples CPU times twice, window apart.
func measure(ctx context.Context, window time.Duration) (map[string]any, error) {
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl); left <= window {
			return nil, fmt.Errorf("invalid params: window %v is not shorter than the rule timeout (%v left); shorten the window or raise timeout", window, left.Round(time.Millisecond))
		}
	}
	a, err := cpuTimes(ctx)
	if err != nil {
		return nil, fmt.Errorf("read CPU times: %w", err)
	}
	if err := sleep(ctx, window); err != nil {
		return nil, fmt.Errorf("measuring CPU usage: %w", err)
	}
	b, err := cpuTimes(ctx)
	if err != nil {
		return nil, fmt.Errorf("read CPU times: %w", err)
	}
	return usageFields(a, b, linux), nil
}
