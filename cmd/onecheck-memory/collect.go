package main

import (
	"context"
	"fmt"
	"math"

	"github.com/shirou/gopsutil/v4/mem"

	"github.com/alekzonder/onecheck/pkg/module"
)

// The system is read through these variables so tests can replace it.
var (
	virtualMemory = mem.VirtualMemoryWithContext
	swapMemory    = mem.SwapMemoryWithContext
)

type params struct{}

func collect(ctx context.Context, req module.Request) (map[string]any, error) {
	var p params
	if err := module.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	vm, err := virtualMemory(ctx)
	if err != nil {
		return nil, fmt.Errorf("read memory: %w", err)
	}
	sw, err := swapMemory(ctx)
	if err != nil {
		return nil, fmt.Errorf("read swap: %w", err)
	}
	return memoryFields(vm.Total, vm.Available, sw.Total, sw.Used, sw.Free), nil
}

// memoryFields builds the module fields. Used memory is total minus
// available, so page cache that can be reclaimed does not count as used.
func memoryFields(total, available, swapTotal, swapUsed, swapFree uint64) map[string]any {
	used := sub(total, available)
	return map[string]any{
		"total":             toInt(total),
		"available":         toInt(available),
		"used":              toInt(used),
		"used_percent":      percent(used, total),
		"swap_total":        toInt(swapTotal),
		"swap_used":         toInt(swapUsed),
		"swap_free":         toInt(swapFree),
		"swap_used_percent": percent(swapUsed, swapTotal),
	}
}

func sub(a, b uint64) uint64 {
	if b > a {
		return 0
	}
	return a - b
}

func toInt(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

// percent is part/whole*100 rounded to two decimals; 0 for an empty whole.
func percent(part, whole uint64) float64 {
	if whole == 0 {
		return 0
	}
	return math.Round(float64(part)/float64(whole)*10000) / 100
}
