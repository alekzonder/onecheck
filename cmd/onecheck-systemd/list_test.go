package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/alekzonder/onecheck/internal/match"
)

const failedList = `nginx.service loaded failed failed A high performance web server
backup@daily.service loaded failed failed Backup of daily data
dev-disk-by\x2dlabel-data.mount loaded failed failed /mnt/data
`

func TestParseUnits(t *testing.T) {
	got := parseUnits("  a.service  loaded failed failed  Some  long description\n\nb.timer not-found inactive dead\n")
	want := []unit{
		{Name: "a.service", LoadState: "loaded", ActiveState: "failed", SubState: "failed", Description: "Some  long description"},
		{Name: "b.timer", LoadState: "not-found", ActiveState: "inactive", SubState: "dead"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("units = %+v", got)
	}
}

func TestCollectFailedUnits(t *testing.T) {
	log := fakeSystemctl(t, map[string]string{"list-units": failedList})
	f, err := run(t, map[string]any{"state": "failed"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(f)
	want := `{"count":3,"names":["backup@daily.service","dev-disk-by\\x2dlabel-data.mount","nginx.service"],"units":[` +
		`{"name":"backup@daily.service","load_state":"loaded","active_state":"failed","sub_state":"failed","description":"Backup of daily data"},` +
		`{"name":"dev-disk-by\\x2dlabel-data.mount","load_state":"loaded","active_state":"failed","sub_state":"failed","description":"/mnt/data"},` +
		`{"name":"nginx.service","load_state":"loaded","active_state":"failed","sub_state":"failed","description":"A high performance web server"}]}`
	if string(data) != want {
		t.Errorf("fields = %s\nwant     %s", data, want)
	}
	c := calls(t, log)
	if len(c) != 1 || !strings.HasPrefix(c[0], "list-units --all --plain --no-legend --no-pager --state=failed XDG=") {
		t.Errorf("calls = %q", c)
	}
}

func TestCollectNothingFailed(t *testing.T) {
	fakeSystemctl(t, map[string]string{"list-units": ""})
	f, err := run(t, map[string]any{"state": "failed"})
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := json.Marshal(f); string(data) != `{"count":0,"names":[],"units":[]}` {
		t.Errorf("fields = %s", data)
	}
}

func TestCollectFailedUnitsExclude(t *testing.T) {
	fakeSystemctl(t, map[string]string{"list-units": failedList})
	f, err := run(t, map[string]any{"state": "failed", "exclude": []string{"backup@*", "*.mount"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f["names"], []string{"nginx.service"}) || f["count"] != 1 || len(f["units"].([]unit)) != 1 {
		t.Errorf("fields = %+v", f)
	}
	if _, err := run(t, map[string]any{"state": "failed", "exclude": []string{"[z-a]"}}); err == nil || !strings.Contains(err.Error(), "exclude") {
		t.Errorf("an invalid exclude pattern must be an error, got %v", err)
	}
}

func TestCollectFailedUnitsOfTheUser(t *testing.T) {
	log := fakeSystemctl(t, map[string]string{"list-units": failedList})
	if _, err := run(t, map[string]any{"state": "failed", "user": true}); err != nil {
		t.Fatal(err)
	}
	if c := calls(t, log); !strings.HasPrefix(c[0], "--user list-units --all ") {
		t.Errorf("call = %q", c[0])
	}
}

func TestListParamsErrors(t *testing.T) {
	fakeSystemctl(t, map[string]string{"list-units": failedList, "nginx": nginxShow})
	for name, params := range map[string]map[string]any{
		"neither unit nor state": {},
		"both":                   {"unit": "nginx", "state": "failed"},
		"exclude with unit":      {"unit": "nginx", "exclude": []string{"a*"}},
		"properties with state":  {"state": "failed", "properties": []string{"MemoryCurrent"}},
		"state is not a name":    {"state": "failed --user"},
	} {
		if _, err := run(t, params); err == nil || !strings.Contains(err.Error(), "invalid params") {
			t.Errorf("%s: err = %v, want invalid params", name, err)
		}
	}
}

func TestListSystemctlFailsIsError(t *testing.T) {
	fakeSystemctl(t, nil)
	if _, err := run(t, map[string]any{"state": "failed"}); err == nil || !strings.Contains(err.Error(), "Failed to connect to bus") {
		t.Errorf("err = %v", err)
	}
}

// The recommended check: "names: []" must fail with a message that names
// the failed units.
func TestFailureOfTheRecommendedCheckNamesTheUnits(t *testing.T) {
	fakeSystemctl(t, map[string]string{"list-units": failedList})
	f, err := run(t, map[string]any{"state": "failed", "exclude": []string{"*.mount"}})
	if err != nil {
		t.Fatal(err)
	}
	types := match.FieldTypes{}
	for name, info := range mod.Description.Fields {
		types[name] = info.Type
	}
	check, err := match.Compile(map[string]any{"names": []any{}}, types, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(f)
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	failures, err := check.Eval(fields, nil)
	if err != nil || len(failures) != 1 {
		t.Fatalf("failures = %+v, err = %v", failures, err)
	}
	if want := `names: expected equals [], got ["backup@daily.service","nginx.service"]`; failures[0].Message != want {
		t.Errorf("message = %s\nwant      %s", failures[0].Message, want)
	}
	fakeSystemctl(t, map[string]string{"list-units": ""})
	f, _ = run(t, map[string]any{"state": "failed"})
	data, _ = json.Marshal(f)
	_ = json.Unmarshal(data, &fields)
	if failures, err := check.Eval(fields, nil); err != nil || len(failures) != 0 {
		t.Errorf("nothing failed must pass: %+v, %v", failures, err)
	}
}
