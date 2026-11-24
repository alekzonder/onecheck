package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"

	"github.com/alekzonder/onecheck/pkg/module"
)

// fakes replaces the system providers; it records calls and sleeps.
type fakes struct {
	timesCalls int
	slept      []time.Duration
	timesErr   error
	countErr   error
	loadErr    error
	sleepErr   error
}

func fakeSystem(t *testing.T) *fakes {
	t.Helper()
	f := &fakes{}
	oldTimes, oldCount, oldLoad, oldSleep := cpuTimes, cpuCount, loadAvg, sleep
	t.Cleanup(func() { cpuTimes, cpuCount, loadAvg, sleep = oldTimes, oldCount, oldLoad, oldSleep })
	cpuTimes = func(context.Context) (cpu.TimesStat, error) {
		f.timesCalls++
		if f.timesErr != nil {
			return cpu.TimesStat{}, f.timesErr
		}
		if f.timesCalls == 1 {
			return cpu.TimesStat{}, nil
		}
		return busySample, nil
	}
	cpuCount = func(context.Context) (int, error) { return 4, f.countErr }
	loadAvg = func(context.Context) (*load.AvgStat, error) {
		if f.loadErr != nil {
			return nil, f.loadErr
		}
		return &load.AvgStat{Load1: 2, Load5: 1, Load15: 0.5}, nil
	}
	sleep = func(_ context.Context, d time.Duration) error {
		f.slept = append(f.slept, d)
		return f.sleepErr
	}
	return f
}

func run(t *testing.T, params map[string]any, fields ...string) (map[string]any, error) {
	t.Helper()
	raw, _ := json.Marshal(params)
	return collect(context.Background(), module.Request{Params: raw, Fields: fields})
}

func TestCollectWaitsWindow(t *testing.T) {
	f := fakeSystem(t)
	out, err := run(t, map[string]any{"window": "2s"}, "usage_percent")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.slept) != 1 || f.slept[0] != 2*time.Second || f.timesCalls != 2 {
		t.Errorf("slept %v, times called %d", f.slept, f.timesCalls)
	}
	if _, ok := out["usage_percent"]; !ok {
		t.Errorf("usage_percent missing: %v", out)
	}
}

func TestCollectLoadOnlyDoesNotWait(t *testing.T) {
	f := fakeSystem(t)
	out, err := run(t, map[string]any{}, "load1")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.slept) != 0 || f.timesCalls != 0 {
		t.Errorf("slept %v, times called %d; want neither", f.slept, f.timesCalls)
	}
	wantFields(t, out, map[string]any{"load1": 2.0, "load1_per_core": 0.5})
}

func TestCollectEmptyFieldsDoesNotWait(t *testing.T) {
	f := fakeSystem(t)
	out, err := run(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.slept) != 0 {
		t.Errorf("slept %v", f.slept)
	}
	wantFields(t, out, map[string]any{"cores": int64(4), "load5": 1.0})
	wantAbsent(t, out, percentFields...)
}

func TestCollectSleepCancelled(t *testing.T) {
	f := fakeSystem(t)
	f.sleepErr = context.Canceled
	if _, err := run(t, nil, "usage_percent"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestCollectBadWindowIsError(t *testing.T) {
	f := fakeSystem(t)
	if _, err := run(t, map[string]any{"window": "abc"}, "usage_percent"); err == nil {
		t.Error("bad window must be an error")
	}
	if f.timesCalls != 0 || len(f.slept) != 0 {
		t.Error("nothing must be read for a bad window")
	}
}

func TestCollectProviderErrors(t *testing.T) {
	for name, set := range map[string]func(*fakes){
		"times": func(f *fakes) { f.timesErr = errors.New("times broke") },
		"count": func(f *fakes) { f.countErr = errors.New("count broke") },
		"load":  func(f *fakes) { f.loadErr = errors.New("load broke") },
	} {
		f := fakeSystem(t)
		set(f)
		_, err := run(t, nil, "usage_percent", "load1")
		if err == nil || !strings.Contains(err.Error(), name+" broke") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestSleepHonoursContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
	if time.Since(start) > time.Second {
		t.Error("sleep ignored the cancelled context")
	}
}

func TestCollectRealCPU(t *testing.T) {
	out, err := run(t, map[string]any{"window": "100ms"}, "usage_percent", "load1")
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := out["usage_percent"].(float64); !ok || p < 0 || p > 100 {
		t.Errorf("usage_percent = %v", out["usage_percent"])
	}
	if c, _ := out["cores"].(int64); c < 1 {
		t.Errorf("cores = %v", out["cores"])
	}
}

func TestDescribeListsFields(t *testing.T) {
	var out bytes.Buffer
	if code := module.Run(mod, []string{"describe"}, strings.NewReader(""), &out, &out); code != 0 {
		t.Fatalf("describe exit %d: %s", code, out.String())
	}
	var d module.Description
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"usage_percent", "user_percent", "system_percent", "iowait_percent", "steal_percent",
		"cores", "load1", "load5", "load15", "load1_per_core", "load5_per_core", "load15_per_core",
	}
	if len(d.Fields) != len(want) {
		t.Errorf("describe has %d fields, want %d", len(d.Fields), len(want))
	}
	for _, k := range want {
		if d.Fields[k].Type != module.TypeNumber {
			t.Errorf("%s type = %q, want number", k, d.Fields[k].Type)
		}
	}
}
