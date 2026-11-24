package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/alekzonder/onecheck/internal/result"
)

func failingCommand(extra string) string {
	return `
  - name: c
    module: command
` + extra + `    params: { run: "echo out; echo err >&2; exit 3" }
    check: { exit_code: 0 }
`
}

func TestEventMarksNoOutputByDefault(t *testing.T) {
	e := newEnv(t)
	e.run(failingCommand(""), Options{})
	if got := string(e.eventJSON()["output_fields"]); got != "[]" {
		t.Errorf("output_fields = %s, want []", got)
	}
}

func TestEventMarksTheOutputARuleAllows(t *testing.T) {
	e := newEnv(t)
	e.run(failingCommand("    event_output: [stderr]\n"), Options{})
	ev := e.events()[0]
	if strings.Join(ev.OutputFields, ",") != "stderr" {
		t.Errorf("output_fields = %v", ev.OutputFields)
	}
	if ev.Result.Output["stdout"] != "out\n" || ev.Result.Output["stderr"] != "err\n" {
		t.Errorf("the mark must not change the saved output: %v", ev.Result.Output)
	}
}

func TestEventParamsFalseHidesOutputToo(t *testing.T) {
	e := newEnv(t)
	e.run(failingCommand("    event_output: [stderr]\n    event_params: false\n"), Options{})
	ev := e.eventJSON()
	if string(ev["output_fields"]) != "[]" || string(ev["redacted"]) != "true" {
		t.Errorf("output_fields = %s, redacted = %s", ev["output_fields"], ev["redacted"])
	}
}

func TestEventOutputMustNameOutputFields(t *testing.T) {
	e := newEnv(t)
	res := e.run(failingCommand("    event_output: [exit_code]\n")+`
  - name: f
    module: file
    event_output: [content]
    params: {path: DATA/f.txt}
`, Options{})
	if res[0].Status != result.Error || !strings.Contains(*res[0].Error, "event_output") ||
		!strings.Contains(*res[0].Error, "exit_code") || !strings.Contains(*res[0].Error, "stderr, stdout") {
		t.Errorf("command: %+v", res[0])
	}
	if res[1].Status != result.Error || !strings.Contains(*res[1].Error, "no output fields") {
		t.Errorf("file: %+v", res[1])
	}
}

func TestEventOfAnUnusableRuleMarksNoOutput(t *testing.T) {
	e := newEnv(t)
	e.run(failingCommand("    event_output: [exit_code]\n")+`
  - name: ghost
    module: nosuch
    event_output: [stderr]
`, Options{})
	evs := e.events()
	if len(evs) != 2 {
		t.Fatalf("want two fail events, got %d", len(evs))
	}
	for _, ev := range evs {
		if ev.OutputFields == nil || len(ev.OutputFields) != 0 {
			t.Errorf("%s: output_fields = %v, want [] for a rule that could not be checked", ev.Rule, ev.OutputFields)
		}
	}
}

func TestEventOfARuleWithAnUnknownKeyStaysRedacted(t *testing.T) {
	e := newEnv(t)
	e.run(`
  - name: typo
    module: command
    event_params: false
    params: { run: "true" }
    chek: { exit_code: 0 }
`, Options{})
	ev := e.eventJSON()
	if string(ev["redacted"]) != "true" || string(ev["params"]) != "null" {
		t.Errorf("redacted = %s, params = %s", ev["redacted"], ev["params"])
	}
}

func TestValidateCatchesParamsThatDoNotGoTogether(t *testing.T) {
	e := newEnv(t)
	cfg := e.load(`
  - {name: ok-disk-path, module: disk, params: {path: /}}
  - {name: ok-disk-all, module: disk}
  - {name: ok-disk-exclude, module: disk, params: {exclude_fstype: [nfs*], exclude_mount: [/mnt/*]}}
  - {name: ok-unit, module: systemd, params: {unit: a.service, properties: [MemoryCurrent]}}
  - {name: ok-state, module: systemd, params: {state: failed, exclude: ["user@*"], user: true}}
  - {name: disk-path-exclude, module: disk, params: {path: /, exclude_mount: [/mnt/*]}}
  - {name: disk-path-fstype, module: disk, params: {path: /, exclude_fstype: [nfs]}}
  - {name: sd-both, module: systemd, params: {unit: a.service, state: failed}}
  - {name: sd-unit-exclude, module: systemd, params: {unit: a.service, exclude: [x]}}
  - {name: sd-state-properties, module: systemd, params: {state: failed, properties: [MemoryCurrent]}}
  - {name: sd-neither, module: systemd, params: {user: true}}
  - {name: sd-typo, module: systemd, params: {state: faild}}
`)
	want := map[string][]string{
		"disk-path-exclude":   {"/exclude_mount", "not allowed"},
		"disk-path-fstype":    {"/exclude_fstype", "not allowed"},
		"sd-both":             {"/unit", "not allowed"},
		"sd-unit-exclude":     {"/exclude", "not allowed"},
		"sd-state-properties": {"/properties", "not allowed"},
		"sd-neither":          {"'unit'", "'state'"},
		"sd-typo":             {"/state"},
	}
	errs := Validate(context.Background(), cfg)
	if len(errs) != len(want) {
		t.Errorf("want %d errors, got %d: %v", len(want), len(errs), errs)
	}
	for _, err := range errs {
		text := err.Error()
		if strings.Contains(text, "oneOf") || strings.Contains(text, "&{") || strings.Contains(text, "false schema") {
			t.Errorf("not worded for a person: %s", text)
		}
		found := false
		for rule, parts := range want {
			if !strings.Contains(text, "rule "+rule+":") {
				continue
			}
			found = true
			for _, part := range parts {
				if !strings.Contains(text, part) {
					t.Errorf("%s: %q lacks %q", rule, text, part)
				}
			}
		}
		if !found {
			t.Errorf("unexpected error: %s", text)
		}
	}
}
