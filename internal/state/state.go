// Package state implements the confirmed pass/fail state of a rule and its
// transitions. It is pure logic: no files, no clocks.
package state

import "time"

// Confirmed states of a rule.
const (
	Pass = "pass"
	Fail = "fail"
)

// Transitions between confirmed states.
const (
	TransitionFail      = "fail"
	TransitionRecovered = "recovered"
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
// TransitionFail or TransitionRecovered.
func Advance(prev *State, ok bool, failAfter, passAfter int, now time.Time) (next State, transition string) {
	if prev == nil {
		next = State{State: Pass, Since: now}
	} else {
		next = *prev
	}
	if ok {
		next.ConsecutivePass++
		next.ConsecutiveFail = 0
	} else {
		next.ConsecutiveFail++
		next.ConsecutivePass = 0
	}
	switch {
	case next.State == Pass && next.ConsecutiveFail >= failAfter:
		next.State, next.Since, transition = Fail, now, TransitionFail
	case next.State == Fail && next.ConsecutivePass >= passAfter:
		next.State, next.Since, transition = Pass, now, TransitionRecovered
	}
	return next, transition
}
