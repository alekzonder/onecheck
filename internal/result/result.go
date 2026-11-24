// Package result defines the JSON contracts onecheck writes to disk and
// prints: the result of one rule run and the state transition event.
package result

import (
	"encoding/json"
	"time"

	"github.com/alekzonder/onecheck/internal/match"
)

// Status is the raw outcome of one run of a rule.
type Status string

// Possible statuses.
const (
	Pass  Status = "pass"  // every condition held
	Fail  Status = "fail"  // at least one condition did not hold
	Error Status = "error" // the check could not be carried out
)

// Result is the outcome of one run of one rule.
type Result struct {
	Config        string   `json:"config"`
	Rule          string   `json:"rule"`
	Module        string   `json:"module"`
	ModuleVersion string   `json:"module_version"`
	Tags          []string `json:"tags"`
	// Status is the outcome of this run.
	Status Status `json:"status"`
	// State is the confirmed state after this run (pass or fail).
	State string `json:"state"`
	// Transition says which event this run wrote: "fail" or "recovered"
	// when it changed the confirmed state, "reminder" when the rule stays
	// failed and a reminder was due, otherwise null.
	Transition *string `json:"transition"`
	// Consecutive is how many runs in a row ended like this one.
	Consecutive int             `json:"consecutive"`
	FailAfter   int             `json:"fail_after"`
	PassAfter   int             `json:"pass_after"`
	StartedAt   time.Time       `json:"started_at"`
	DurationMS  int64           `json:"duration_ms"`
	Failures    []match.Failure `json:"failures"`
	Error       *string         `json:"error"`
	// Output holds the tail of the module's output fields (a command's
	// stdout and stderr, a response body), kept for diagnosis.
	Output map[string]any `json:"output"`
}

// MarshalJSON writes empty lists as [] rather than null.
func (r Result) MarshalJSON() ([]byte, error) {
	type plain Result
	p := plain(r)
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if p.Failures == nil {
		p.Failures = []match.Failure{}
	}
	if p.Output == nil {
		p.Output = map[string]any{}
	}
	return json.Marshal(p)
}

// OK reports whether the run passed.
func (r Result) OK() bool { return r.Status == Pass }

// Event is written to the event queue when a rule's confirmed state changes
// and, as a reminder, while it stays failed.
type Event struct {
	ID         string    `json:"id"`
	Config     string    `json:"config"`
	Rule       string    `json:"rule"`
	Transition string    `json:"transition"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	At         time.Time `json:"at"`
	ExpiresAt  time.Time `json:"expires_at"`
	// Since is when the confirmed state the event reports (To) began: At
	// for a change of state, the start of the problem for a reminder.
	Since time.Time `json:"since"`
	// Params is the rule's params as written in the config, before ${VAR}
	// expansion; null when the rule has none or hides them.
	Params map[string]any `json:"params"`
	// Redacted is true when the rule has event_params: false: a consumer
	// must not show the details of the result to third parties.
	Redacted bool `json:"redacted"`
	// OutputFields names the keys of Result.Output the rule allows a
	// consumer to show (event_output); empty by default and when Redacted.
	OutputFields []string `json:"output_fields"`
	Result       Result   `json:"result"`
}
