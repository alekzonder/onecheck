package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"runtime"
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

type params struct {
	Window string `json:"window"`
}

func collect(ctx context.Context, req module.Request) (map[string]any, error) {
	var p params
	if err := module.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	window, err := parseWindow(p.Window)
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
		if req.WantsOr(f, false) {
			return true
		}
	}
	return false
}

// measure samples CPU times twice, window apart.
func measure(ctx context.Context, window time.Duration) (map[string]any, error) {
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
	return usageFields(a, b, runtime.GOOS == "linux"), nil
}
