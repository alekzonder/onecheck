// Package state implements the confirmed pass/fail state of a rule and its
// transitions. It is pure logic: no files, no clocks.
package state

import "time"

// Confirmed states of a rule.
const (
	Pass = "pass"
	Fail = "fail"
)

// Transitions between confirmed states, and the reminder: a rule that stays
// failed says so again without changing its state.
const (
	TransitionFail      = "fail"
	TransitionRecovered = "recovered"
	TransitionReminder  = "reminder"
)

// State is what onecheck remembers about a rule between runs.
type State struct {
	// State is the confirmed state: it changes only after fail_after
	// consecutive failures or pass_after consecutive passes.
	State string `json:"state"`
	// Since is when the confirmed state last changed.
	Since           time.Time `json:"since"`
	ConsecutivePass int       `json:"consecutive_pass"`
	ConsecutiveFail int       `json:"consecutive_fail"`
	// RemindedAt is when the last reminder of the current failure was
	// written; zero when there was none.
	RemindedAt time.Time `json:"reminded_at,omitzero"`
	// CheckedAt is when the rule last ran; zero in a state written by a
	// version that did not record it.
	CheckedAt time.Time `json:"checked_at,omitzero"`
}

// Consecutive returns the length of the current streak of passes (ok) or
// failures (!ok).
func (s State) Consecutive(ok bool) int {
	if ok {
		return s.ConsecutivePass
	}
	return s.ConsecutiveFail
}

// Advance applies the outcome of one run. A rule seen for the first time
// (prev == nil) starts in the pass state. The returned transition is "",
// TransitionFail, TransitionRecovered or TransitionReminder.
//
// A reminder is due every remindEvery (0 = never) counted from the moment
// the state became fail, and is returned by the first run at a due time
// (see reminderDue) that did not pass: a run that passed while the state is still fail has
// nothing to remind of.
func Advance(prev *State, ok bool, failAfter, passAfter int, remindEvery time.Duration, now time.Time) (next State, transition string) {
	if prev == nil {
		next = State{State: Pass, Since: now}
	} else {
		next = *prev
	}
	next.CheckedAt = now
	if ok {
		next.ConsecutivePass++
		next.ConsecutiveFail = 0
	} else {
		next.ConsecutiveFail++
		next.ConsecutivePass = 0
	}
	switch {
	case next.State == Pass && next.ConsecutiveFail >= failAfter:
		next.State, next.Since, next.RemindedAt, transition = Fail, now, time.Time{}, TransitionFail
	case next.State == Fail && next.ConsecutivePass >= passAfter:
		next.State, next.Since, next.RemindedAt, transition = Pass, now, time.Time{}, TransitionRecovered
	case next.State == Fail && !ok && reminderDue(next, remindEvery, now):
		next.RemindedAt, transition = now, TransitionReminder
	}
	return next, transition
}

// reminderDue reports whether a due time (since + N*remindEvery, N >= 1) has
// passed that no reminder was written for yet. Counting from since keeps
// late runs from shifting the schedule, and a long gap gives one reminder.
//
// A run up to a tenth of the interval early counts as on time: since is
// itself the time of a run, so a config run every remindEvery lands a moment
// before or after each due time, and the early runs must not be skipped.
func reminderDue(s State, remindEvery time.Duration, now time.Time) bool {
	if remindEvery <= 0 {
		return false
	}
	n := (now.Sub(s.Since) + remindEvery/10) / remindEvery
	return n >= 1 && s.Since.Add(n*remindEvery).After(s.RemindedAt)
}
