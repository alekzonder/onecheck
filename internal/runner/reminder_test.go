package runner

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/onecheck/internal/result"
)

// goneRule fails on its first run: the file it wants does not exist.
func goneRule(extra string) string {
	return `
  - name: gone
    module: file
` + extra + `    params: {path: DATA/missing.txt}
    check: {exists: true}
`
}

func (e *env) stateFile(rule string) map[string]any {
	e.t.Helper()
	var st map[string]any
	if err := json.Unmarshal(mustRead(e.t, filepath.Join(e.dir, "state", "main", rule+".json")), &st); err != nil {
		e.t.Fatal(err)
	}
	return st
}

// backdate moves every time in the rule's state file back by d, as if the
// runs so far had happened that much earlier.
func (e *env) backdate(rule string, d time.Duration) {
	e.t.Helper()
	st := e.stateFile(rule)
	for _, key := range []string{"since", "reminded_at", "checked_at"} {
		text, ok := st[key].(string)
		if !ok {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			e.t.Fatal(err)
		}
		st[key] = at.Add(-d).Format(time.RFC3339Nano)
	}
	e.writeState(rule, st)
}

func (e *env) writeState(rule string, st map[string]any) {
	e.t.Helper()
	data, err := json.Marshal(st)
	if err != nil {
		e.t.Fatal(err)
	}
	dir := filepath.Join(e.dir, "state", "main")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rule+".json"), data, 0o640); err != nil {
		e.t.Fatal(err)
	}
}

// byTransition returns the events of the env with that transition.
func (e *env) byTransition(transition string) []result.Event {
	e.t.Helper()
	var out []result.Event
	for _, ev := range e.events() {
		if ev.Transition == transition {
			out = append(out, ev)
		}
	}
	return out
}

// runLogged runs the rules and returns what was logged at warn and above.
func (e *env) runLogged(rules string) string {
	e.t.Helper()
	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if _, err := Run(context.Background(), e.load(rules), Options{}, log); err != nil {
		e.t.Fatalf("Run: %v", err)
	}
	return buf.String()
}

func TestReminderIsWrittenWhenDue(t *testing.T) {
	e := newEnv(t)
	rules := goneRule("    remind_every: 1h\n")
	e.run(rules, Options{})
	if res := e.run(rules, Options{}); res[0].Transition != nil || len(e.events()) != 1 {
		t.Fatalf("no reminder is due yet: %+v, events=%d", res[0], len(e.events()))
	}
	failed := e.byTransition("fail")[0]
	e.backdate("gone", 61*time.Minute)
	res := e.run(rules, Options{})
	if res[0].Transition == nil || *res[0].Transition != "reminder" || res[0].State != "fail" {
		t.Fatalf("result of the reminding run = %+v", res[0])
	}
	reminders := e.byTransition("reminder")
	if len(reminders) != 1 || len(e.events()) != 2 {
		t.Fatalf("want one reminder next to the fail event, events = %+v", e.events())
	}
	ev := reminders[0]
	if ev.From != "fail" || ev.To != "fail" || !strings.HasSuffix(ev.ID, "-main-gone-reminder") || ev.Rule != "gone" || ev.Config != "main" {
		t.Errorf("event = %+v", ev)
	}
	if want := failed.At.Add(-61 * time.Minute); !ev.Since.Equal(want) {
		t.Errorf("since = %s, want the start of the problem %s", ev.Since, want)
	}
	if ev.Result.Status != result.Fail || len(ev.Result.Failures) != 1 || ev.ExpiresAt.Sub(ev.At) != 24*time.Hour {
		t.Errorf("a reminder carries the current result and expires like any event: %+v", ev)
	}
	if saved := e.saved("gone"); saved.Transition == nil || *saved.Transition != "reminder" {
		t.Errorf("saved result = %+v", saved)
	}
	if e.run(rules, Options{}); len(e.events()) != 2 {
		t.Errorf("the next run must not remind again, events = %d", len(e.events()))
	}
}

func TestFailAndRecoveredEventsCarrySince(t *testing.T) {
	e := newEnv(t)
	rules := `
  - name: f
    module: file
    params: {path: DATA/f.txt}
    check: {exists: true}
`
	e.run(rules, Options{})
	e.touch("f.txt", "x")
	e.run(rules, Options{})
	evs := e.events()
	if len(evs) != 2 {
		t.Fatalf("want a fail and a recovered event, got %d", len(evs))
	}
	for _, ev := range evs {
		if ev.Since.IsZero() || !ev.Since.Equal(ev.At) {
			t.Errorf("%s: since = %s, at = %s; a change of state begins when it is recorded", ev.Transition, ev.Since, ev.At)
		}
	}
}

func TestReminderRespectsEventParams(t *testing.T) {
	e := newEnv(t)
	rules := goneRule("    event_params: false\n")
	e.run(rules, Options{})
	e.backdate("gone", 61*time.Minute)
	e.run(rules, Options{})
	reminders := e.byTransition("reminder")
	if len(reminders) != 1 || reminders[0].Params != nil || !reminders[0].Redacted {
		t.Errorf("reminders = %+v", reminders)
	}
}

func TestReminderOffWithZero(t *testing.T) {
	e := newEnv(t)
	rules := goneRule("    remind_every: 0\n")
	e.run(rules, Options{})
	e.backdate("gone", 5*time.Hour)
	if res := e.run(rules, Options{}); res[0].Transition != nil || len(e.events()) != 1 {
		t.Errorf("remind_every: 0 must write no reminder: %+v, events=%d", res[0], len(e.events()))
	}
}

func TestReminderOfAnErrorRun(t *testing.T) {
	e := newEnv(t)
	e.run(goneRule(""), Options{})
	e.backdate("gone", 61*time.Minute)
	e.run("\n  - name: gone\n    module: nosuch\n", Options{})
	reminders := e.byTransition("reminder")
	if len(reminders) != 1 || reminders[0].Result.Status != result.Error || reminders[0].Result.Error == nil {
		t.Errorf("a rule in fail whose check cannot run is reminded of too: %+v", reminders)
	}
}

func TestReminderForAStateFileOfAnOlderVersion(t *testing.T) {
	e := newEnv(t)
	since := time.Now().UTC().Add(-3 * time.Hour)
	e.writeState("gone", map[string]any{
		"state": "fail", "since": since.Format(time.RFC3339Nano), "consecutive_pass": 0, "consecutive_fail": 2,
	})
	log := e.runLogged(goneRule("    remind_every: 1m\n"))
	reminders := e.byTransition("reminder")
	if len(reminders) != 1 || !reminders[0].Since.Equal(since) {
		t.Errorf("reminders = %+v", reminders)
	}
	if log != "" {
		t.Errorf("the previous run is unknown, nothing to compare remind_every with:\n%s", log)
	}
}

func TestReminderEventWriteFailureIsRetried(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	e := newEnv(t)
	rules := goneRule("")
	e.run(rules, Options{})
	e.backdate("gone", 61*time.Minute)
	events := filepath.Join(e.dir, "events")
	if err := os.Chmod(events, 0o555); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), e.load(rules), Options{}, quiet)
	if cerr := os.Chmod(events, 0o755); cerr != nil {
		t.Fatal(cerr)
	}
	if err == nil {
		t.Fatal("a reminder that cannot be written must fail the run")
	}
	if _, reminded := e.stateFile("gone")["reminded_at"]; reminded {
		t.Error("the state must not record a reminder that was not written")
	}
	if e.run(rules, Options{}); len(e.byTransition("reminder")) != 1 {
		t.Errorf("the next run must write the reminder, events = %+v", e.events())
	}
}

func TestRemindShorterThanRuns(t *testing.T) {
	cases := []struct {
		remindEvery, sinceLastRun time.Duration
		want                      bool
	}{
		{time.Minute, 60400 * time.Millisecond, false}, // the same interval, a late start
		{time.Minute, 66 * time.Second, false},
		{time.Minute, 67 * time.Second, true},
		{time.Minute, 5 * time.Minute, true},
		{time.Hour, time.Minute, false},
	}
	for _, c := range cases {
		if got := remindShorterThanRuns(c.remindEvery, c.sinceLastRun); got != c.want {
			t.Errorf("remindShorterThanRuns(%s, %s) = %v, want %v", c.remindEvery, c.sinceLastRun, got, c.want)
		}
	}
}

func TestShortRemindEveryIsReported(t *testing.T) {
	t.Run("set in the config", func(t *testing.T) {
		e := newEnv(t)
		rules := goneRule("    remind_every: 1m\n")
		e.run(rules, Options{})
		e.backdate("gone", 10*time.Minute)
		log := e.runLogged(rules)
		if !strings.Contains(log, "level=WARN") || !strings.Contains(log, "remind_every") || !strings.Contains(log, "rule=gone") {
			t.Errorf("want a warning about remind_every of the rule, got:\n%s", log)
		}
		if len(e.byTransition("reminder")) != 1 {
			t.Error("the reminder is still written")
		}
	})
	t.Run("built-in default", func(t *testing.T) {
		e := newEnv(t)
		rules := goneRule("")
		e.run(rules, Options{})
		e.backdate("gone", 3*time.Hour)
		if log := e.runLogged(rules); log != "" {
			t.Errorf("a config that never set remind_every must not be warned about it:\n%s", log)
		}
		if len(e.byTransition("reminder")) != 1 {
			t.Error("the reminder is still written")
		}
	})
}

// rawEvents returns the event files of that transition as plain JSON objects,
// to see which keys are written.
func (e *env) rawEvents(transition string) []map[string]any {
	e.t.Helper()
	files, _ := filepath.Glob(filepath.Join(e.dir, "events", "*-"+transition+".json"))
	var out []map[string]any
	for _, f := range files {
		var ev map[string]any
		if err := json.Unmarshal(mustRead(e.t, f), &ev); err != nil {
			e.t.Fatalf("%s: %v", f, err)
		}
		out = append(out, ev)
	}
	return out
}

func TestRecoveredEventCarriesFailedSince(t *testing.T) {
	e := newEnv(t)
	rules := goneRule("")
	e.run(rules, Options{})
	e.touch("missing.txt", "x")
	e.run(rules, Options{})
	failed, recovered := e.byTransition("fail"), e.byTransition("recovered")
	if len(failed) != 1 || len(recovered) != 1 {
		t.Fatalf("events = %+v", e.events())
	}
	if !recovered[0].FailedSince.Equal(failed[0].At) {
		t.Errorf("failed_since = %s, want the time of the fail event %s", recovered[0].FailedSince, failed[0].At)
	}
	if !recovered[0].Since.Equal(recovered[0].At) {
		t.Errorf("since of recovered = %s, must stay equal to at %s", recovered[0].Since, recovered[0].At)
	}
}

func TestFailedSinceSurvivesReminders(t *testing.T) {
	e := newEnv(t)
	rules := goneRule("    remind_every: 1h\n")
	e.run(rules, Options{})
	failedAt := e.byTransition("fail")[0].At
	for i := range 2 {
		e.backdate("gone", 61*time.Minute)
		if res := e.run(rules, Options{}); res[0].Transition == nil || *res[0].Transition != "reminder" {
			t.Fatalf("run %d: want a reminder, got %+v", i+1, res[0])
		}
	}
	// Both reminders fall in one second and share an event ID, so one file
	// is left; it is enough to compare with.
	reminders := e.byTransition("reminder")
	e.touch("missing.txt", "x")
	e.run(rules, Options{})
	recovered := e.byTransition("recovered")
	if len(recovered) != 1 {
		t.Fatalf("events = %+v", e.events())
	}
	if want := failedAt.Add(-122 * time.Minute); !recovered[0].FailedSince.Equal(want) {
		t.Errorf("failed_since = %s, want the start of the failure %s", recovered[0].FailedSince, want)
	}
	for _, r := range reminders {
		if recovered[0].FailedSince.Equal(r.At) {
			t.Errorf("failed_since is the time of a reminder %s", r.At)
		}
	}
}

func TestFailedSinceOnlyInRecovered(t *testing.T) {
	e := newEnv(t)
	rules := goneRule("    remind_every: 1h\n")
	e.run(rules, Options{})
	e.backdate("gone", 61*time.Minute)
	e.run(rules, Options{})
	e.touch("missing.txt", "x")
	e.run(rules, Options{})
	for _, transition := range []string{"fail", "reminder"} {
		evs := e.rawEvents(transition)
		if len(evs) != 1 {
			t.Fatalf("%s events = %v", transition, evs)
		}
		if _, ok := evs[0]["failed_since"]; ok {
			t.Errorf("%s event has failed_since: %v", transition, evs[0])
		}
	}
	if evs := e.rawEvents("recovered"); len(evs) != 1 || evs[0]["failed_since"] == nil {
		t.Errorf("recovered event has no failed_since: %v", evs)
	}
}

func TestFailedSinceWhenRedacted(t *testing.T) {
	e := newEnv(t)
	rules := goneRule("    event_params: false\n")
	e.run(rules, Options{})
	e.touch("missing.txt", "x")
	e.run(rules, Options{})
	recovered := e.byTransition("recovered")
	if len(recovered) != 1 || !recovered[0].Redacted || recovered[0].FailedSince.IsZero() {
		t.Errorf("a redacted recovered event still carries failed_since: %+v", recovered)
	}
}

func TestRecoveredFromAStateFileOfAnOlderVersion(t *testing.T) {
	e := newEnv(t)
	since := time.Now().UTC().Add(-3 * time.Hour)
	e.writeState("gone", map[string]any{
		"state": "fail", "since": since.Format(time.RFC3339Nano), "consecutive_pass": 0, "consecutive_fail": 2,
	})
	e.touch("missing.txt", "x")
	e.run(goneRule(""), Options{})
	recovered := e.byTransition("recovered")
	if len(recovered) != 1 || !recovered[0].FailedSince.Equal(since) {
		t.Errorf("recovered = %+v, want failed_since %s", recovered, since)
	}
}
