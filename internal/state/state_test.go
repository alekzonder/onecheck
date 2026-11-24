package state

import (
	"testing"
	"time"
)

func TestAdvance(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	type step struct {
		ok         bool
		wantState  string
		transition string
	}
	cases := []struct {
		name                 string
		failAfter, passAfter int
		steps                []step
	}{
		{"new rule passing is silent", 1, 1, []step{{true, Pass, ""}, {true, Pass, ""}}},
		{"new rule failing emits fail", 1, 1, []step{{false, Fail, TransitionFail}, {false, Fail, ""}}},
		{"fail_after 3 needs three in a row", 3, 1, []step{
			{false, Pass, ""}, {false, Pass, ""}, {false, Fail, TransitionFail}, {false, Fail, ""},
		}},
		{"a pass in between resets the counter", 3, 1, []step{
			{false, Pass, ""}, {false, Pass, ""}, {true, Pass, ""}, {false, Pass, ""}, {false, Pass, ""}, {false, Fail, TransitionFail},
		}},
		{"recovery needs pass_after in a row", 1, 2, []step{
			{false, Fail, TransitionFail}, {true, Fail, ""}, {false, Fail, ""}, {true, Fail, ""}, {true, Pass, TransitionRecovered}, {true, Pass, ""},
		}},
		{"flapping with thresholds 1/1", 1, 1, []step{
			{false, Fail, TransitionFail}, {true, Pass, TransitionRecovered}, {false, Fail, TransitionFail},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var prev *State
			for i, s := range c.steps {
				next, tr := Advance(prev, s.ok, c.failAfter, c.passAfter, 0, now.Add(time.Duration(i)*time.Minute))
				if next.State != s.wantState || tr != s.transition {
					t.Fatalf("step %d: state=%q transition=%q, want %q %q", i, next.State, tr, s.wantState, s.transition)
				}
				prev = &next
			}
		})
	}
}

func TestAdvanceCountersAndSince(t *testing.T) {
	t0 := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	s1, _ := Advance(nil, true, 2, 1, 0, t0)
	if s1.ConsecutivePass != 1 || s1.ConsecutiveFail != 0 || !s1.Since.Equal(t0) {
		t.Errorf("s1 = %+v", s1)
	}
	s2, _ := Advance(&s1, false, 2, 1, 0, t0.Add(time.Minute))
	if s2.ConsecutivePass != 0 || s2.ConsecutiveFail != 1 || !s2.Since.Equal(t0) {
		t.Errorf("s2 = %+v", s2)
	}
	s3, tr := Advance(&s2, false, 2, 1, 0, t0.Add(2*time.Minute))
	if tr != TransitionFail || s3.ConsecutiveFail != 2 || !s3.Since.Equal(t0.Add(2*time.Minute)) {
		t.Errorf("s3 = %+v tr=%q; since must move on a transition", s3, tr)
	}
	if s3.Consecutive(false) != 2 || s3.Consecutive(true) != 0 {
		t.Errorf("Consecutive = %d/%d", s3.Consecutive(false), s3.Consecutive(true))
	}
}

func TestAdvanceReminders(t *testing.T) {
	t0 := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	const m, h = time.Minute, time.Hour
	type step struct {
		at   time.Duration
		ok   bool
		want string
	}
	cases := []struct {
		name                 string
		failAfter, passAfter int
		remindEvery          time.Duration
		steps                []step
	}{
		{"due times count from the start", 1, 1, h, []step{
			{0, false, TransitionFail}, {53 * m, false, ""}, {60 * m, false, TransitionReminder}, {61 * m, false, ""},
			{113 * m, false, ""}, {121 * m, false, TransitionReminder},
		}},
		{"late run does not shift the schedule", 1, 1, h, []step{
			{0, false, TransitionFail}, {90 * m, false, TransitionReminder}, {120 * m, false, TransitionReminder},
		}},
		{"one reminder after a long gap", 1, 1, h, []step{
			{0, false, TransitionFail}, {10*h + 30*m, false, TransitionReminder}, {10*h + 31*m, false, ""}, {11 * h, false, TransitionReminder},
		}},
		{"a passing run is not reminded, the next failing one is", 1, 3, h, []step{
			{0, false, TransitionFail}, {60 * m, true, ""}, {61 * m, false, TransitionReminder},
		}},
		{"recovery ends reminders", 1, 1, h, []step{
			{0, false, TransitionFail}, {30 * m, true, TransitionRecovered}, {2 * h, true, ""},
		}},
		{"a new failure starts a new schedule", 1, 1, h, []step{
			{0, false, TransitionFail}, {70 * m, false, TransitionReminder}, {80 * m, true, TransitionRecovered},
			{90 * m, false, TransitionFail}, {100 * m, false, ""}, {150 * m, false, TransitionReminder},
		}},
		{"off", 1, 1, 0, []step{{0, false, TransitionFail}, {5 * h, false, ""}}},
		{"not confirmed yet", 3, 1, h, []step{{0, false, ""}, {2 * h, false, ""}}},
		{"clock moved back", 1, 1, h, []step{
			{0, false, TransitionFail}, {-2 * h, false, ""}, {70 * m, false, TransitionReminder}, {30 * m, false, ""},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var prev *State
			for i, s := range c.steps {
				next, tr := Advance(prev, s.ok, c.failAfter, c.passAfter, c.remindEvery, t0.Add(s.at))
				if tr != s.want {
					t.Fatalf("step %d (at %s): transition=%q, want %q", i, s.at, tr, s.want)
				}
				prev = &next
			}
		})
	}
}

func TestAdvanceReminderBookkeeping(t *testing.T) {
	t0 := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	s1, _ := Advance(nil, false, 1, 1, time.Hour, t0)
	if !s1.RemindedAt.IsZero() || !s1.CheckedAt.Equal(t0) {
		t.Errorf("after fail: %+v", s1)
	}
	at := t0.Add(70 * time.Minute)
	s2, tr := Advance(&s1, false, 1, 1, time.Hour, at)
	if tr != TransitionReminder || !s2.RemindedAt.Equal(at) || !s2.Since.Equal(t0) || !s2.CheckedAt.Equal(at) || s2.State != Fail {
		t.Errorf("after reminder: %+v tr=%q; a reminder must not move since", s2, tr)
	}
	at = t0.Add(80 * time.Minute)
	s3, tr := Advance(&s2, true, 1, 1, time.Hour, at)
	if tr != TransitionRecovered || !s3.RemindedAt.IsZero() || !s3.CheckedAt.Equal(at) {
		t.Errorf("after recovery: %+v tr=%q", s3, tr)
	}
}

func TestAdvanceRemindsAStateOfAnOlderVersion(t *testing.T) {
	t0 := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	old := &State{State: Fail, Since: t0, ConsecutiveFail: 4}
	if _, tr := Advance(old, false, 1, 1, time.Hour, t0.Add(3*time.Hour)); tr != TransitionReminder {
		t.Errorf("transition = %q, want a reminder", tr)
	}
}

// A config run at the same interval as remind_every never lands exactly on
// the due times: every run but the first must still write its reminder.
func TestAdvanceRemindsOnEveryRunOfTheSameInterval(t *testing.T) {
	t0 := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	jitter := []time.Duration{0, 300, -200, 400, -350, 50, -400, 0, 250, -100} // milliseconds
	for _, every := range []time.Duration{time.Minute, time.Hour} {
		s, _ := Advance(nil, false, 1, 1, every, t0)
		for i := 1; i <= 200; i++ {
			at := t0.Add(time.Duration(i)*every + jitter[i%len(jitter)]*time.Millisecond)
			next, tr := Advance(&s, false, 1, 1, every, at)
			if tr != TransitionReminder {
				t.Fatalf("remind_every %s, run %d at %s: transition=%q, want a reminder", every, i, at.Sub(t0), tr)
			}
			s = next
		}
	}
}

func TestAdvanceRemindsSlightlyEarlyRun(t *testing.T) {
	t0 := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	s, _ := Advance(nil, false, 1, 1, time.Hour, t0)
	if _, tr := Advance(&s, false, 1, 1, time.Hour, t0.Add(53*time.Minute)); tr != "" {
		t.Errorf("53m of 1h: transition=%q, want none", tr)
	}
	if _, tr := Advance(&s, false, 1, 1, time.Hour, t0.Add(54*time.Minute)); tr != TransitionReminder {
		t.Errorf("54m of 1h is within a tenth of the interval: transition=%q, want a reminder", tr)
	}
}
