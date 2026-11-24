package runner

import (
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
