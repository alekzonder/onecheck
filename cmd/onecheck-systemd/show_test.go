package main

import (
	"testing"
	"time"
)

func TestParseShowSplitsOnFirstEquals(t *testing.T) {
	p := parseShow("ExecStart={ path=/bin/x ; argv[]=/bin/x a=b }\nUnitFileState=\nnoequals\n")
	if len(p) != 2 {
		t.Fatalf("parsed %d keys, want 2: %v", len(p), p)
	}
	if p["ExecStart"] != "{ path=/bin/x ; argv[]=/bin/x a=b }" {
		t.Errorf("ExecStart = %q", p["ExecStart"])
	}
	if v, ok := p["UnitFileState"]; !ok || v != "" {
		t.Errorf("UnitFileState = %q, %v; want empty and present", v, ok)
	}
}

func TestParseTime(t *testing.T) {
	got, ok := parseTime("Tue 2026-11-24 03:34:19 UTC")
	if !ok || !got.Equal(time.Date(2026, 11, 24, 3, 34, 19, 0, time.UTC)) {
		t.Errorf("parseTime = %v, %v", got, ok)
	}
	for _, v := range []string{"", "n/a", "0"} {
		if _, ok := parseTime(v); ok {
			t.Errorf("parseTime(%q) ok, want false", v)
		}
	}
}
