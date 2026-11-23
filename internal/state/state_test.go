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
				next, tr := Advance(prev, s.ok, c.failAfter, c.passAfter, now.Add(time.Duration(i)*time.Minute))
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
	s1, _ := Advance(nil, true, 2, 1, t0)
	if s1.ConsecutivePass != 1 || s1.ConsecutiveFail != 0 || !s1.Since.Equal(t0) {
		t.Errorf("s1 = %+v", s1)
	}
	s2, _ := Advance(&s1, false, 2, 1, t0.Add(time.Minute))
	if s2.ConsecutivePass != 0 || s2.ConsecutiveFail != 1 || !s2.Since.Equal(t0) {
		t.Errorf("s2 = %+v", s2)
	}
	s3, tr := Advance(&s2, false, 2, 1, t0.Add(2*time.Minute))
	if tr != TransitionFail || s3.ConsecutiveFail != 2 || !s3.Since.Equal(t0.Add(2*time.Minute)) {
		t.Errorf("s3 = %+v tr=%q; since must move on a transition", s3, tr)
	}
	if s3.Consecutive(false) != 2 || s3.Consecutive(true) != 0 {
		t.Errorf("Consecutive = %d/%d", s3.Consecutive(false), s3.Consecutive(true))
	}
}
