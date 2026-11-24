package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shirou/gopsutil/v4/mem"

	"github.com/alekzonder/onecheck/pkg/module"
)

func wantFields(t *testing.T, got, want map[string]any) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[k], v)
		}
	}
}

func TestMemoryFields(t *testing.T) {
	f := memoryFields(16<<30, 4<<30, 2<<30, 1<<29, 3<<29)
	wantFields(t, f, map[string]any{
		"total": int64(16 << 30), "available": int64(4 << 30), "used": int64(12 << 30), "used_percent": 75.0,
		"swap_total": int64(2 << 30), "swap_used": int64(1 << 29), "swap_free": int64(3 << 29), "swap_used_percent": 25.0,
	})
	if len(f) != 8 {
		t.Errorf("%d fields, want 8: %v", len(f), f)
	}
}

func TestMemoryFieldsNoSwap(t *testing.T) {
	f := memoryFields(8<<30, 2<<30, 0, 0, 0)
	wantFields(t, f, map[string]any{"swap_total": int64(0), "swap_used": int64(0), "swap_free": int64(0), "swap_used_percent": 0.0})
}

func TestMemoryAvailableAboveTotal(t *testing.T) {
	f := memoryFields(1<<30, 2<<30, 0, 0, 0)
	wantFields(t, f, map[string]any{"used": int64(0), "used_percent": 0.0})
}

func fakeProviders(t *testing.T, vm *mem.VirtualMemoryStat, vmErr error, sw *mem.SwapMemoryStat, swErr error) {
	t.Helper()
	oldVM, oldSW := virtualMemory, swapMemory
	t.Cleanup(func() { virtualMemory, swapMemory = oldVM, oldSW })
	virtualMemory = func(context.Context) (*mem.VirtualMemoryStat, error) { return vm, vmErr }
	swapMemory = func(context.Context) (*mem.SwapMemoryStat, error) { return sw, swErr }
}

func run(t *testing.T, params string) (map[string]any, error) {
	t.Helper()
	return collect(context.Background(), module.Request{Params: json.RawMessage(params)})
}

func TestCollectUsesProviders(t *testing.T) {
	fakeProviders(t, &mem.VirtualMemoryStat{Total: 1000, Available: 250, Used: 1}, nil,
		&mem.SwapMemoryStat{Total: 100, Used: 10, Free: 90}, nil)
	f, err := run(t, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	wantFields(t, f, map[string]any{"used": int64(750), "used_percent": 75.0, "swap_used_percent": 10.0})
}

func TestCollectMemoryErrorIsError(t *testing.T) {
	fakeProviders(t, nil, errors.New("no meminfo"), &mem.SwapMemoryStat{}, nil)
	if _, err := run(t, `{}`); err == nil || !strings.Contains(err.Error(), "no meminfo") {
		t.Errorf("err = %v", err)
	}
}

func TestCollectSwapErrorIsError(t *testing.T) {
	fakeProviders(t, &mem.VirtualMemoryStat{Total: 1}, nil, nil, errors.New("no swap info"))
	if _, err := run(t, `{}`); err == nil || !strings.Contains(err.Error(), "no swap info") {
		t.Errorf("err = %v", err)
	}
}

func TestCollectRejectsParams(t *testing.T) {
	fakeProviders(t, &mem.VirtualMemoryStat{Total: 1}, nil, &mem.SwapMemoryStat{}, nil)
	if _, err := run(t, `{"x":1}`); err == nil {
		t.Error("unknown param must be an error")
	}
}

func TestCollectRealMemory(t *testing.T) {
	f, err := run(t, ``)
	if err != nil {
		t.Fatal(err)
	}
	if total, _ := f["total"].(int64); total <= 0 {
		t.Errorf("total = %v", f["total"])
	}
	if p, ok := f["used_percent"].(float64); !ok || p < 0 || p > 100 {
		t.Errorf("used_percent = %v", f["used_percent"])
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
	want := map[string]module.FieldType{
		"total": module.TypeBytes, "available": module.TypeBytes, "used": module.TypeBytes, "used_percent": module.TypeNumber,
		"swap_total": module.TypeBytes, "swap_used": module.TypeBytes, "swap_free": module.TypeBytes, "swap_used_percent": module.TypeNumber,
	}
	if len(d.Fields) != len(want) {
		t.Errorf("describe has %d fields, want %d", len(d.Fields), len(want))
	}
	for k, typ := range want {
		if d.Fields[k].Type != typ {
			t.Errorf("%s type = %q, want %q", k, d.Fields[k].Type, typ)
		}
	}
}
