// Package output renders run results for people (text) and programs (json).
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/alekzonder/onecheck/internal/result"
)

// Summary counts the results of a run. It always covers every rule that ran,
// also when only failures are shown.
type Summary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
	Errors int `json:"errors"`
}

// Summarize counts results by status.
func Summarize(results []result.Result) Summary {
	s := Summary{Total: len(results)}
	for _, r := range results {
		switch r.Status {
		case result.Pass:
			s.Passed++
		case result.Fail:
			s.Failed++
		default:
			s.Errors++
		}
	}
	return s
}

// ExitCode is 0 when every result passed and 1 otherwise. It reflects the
// raw status of this run, not the confirmed state.
func ExitCode(results []result.Result) int {
	for _, r := range results {
		if !r.OK() {
			return 1
		}
	}
	return 0
}

// Write renders the results in the given format ("text" or "json"). With
// onlyFailed, passing rules are left out of the list but not of the summary.
func Write(w io.Writer, format string, onlyFailed bool, results []result.Result) error {
	shown := []result.Result{}
	for _, r := range results {
		if !onlyFailed || !r.OK() {
			shown = append(shown, r)
		}
	}
	summary := Summarize(results)
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Summary Summary         `json:"summary"`
			Results []result.Result `json:"results"`
		}{summary, shown})
	case "text":
		return writeText(w, summary, shown)
	}
	return fmt.Errorf("unknown output format %q (expected text or json)", format)
}

func writeText(w io.Writer, summary Summary, shown []result.Result) error {
	width := 0
	for _, r := range shown {
		width = max(width, len(r.Rule))
	}
	var b strings.Builder
	for _, r := range shown {
		fmt.Fprintf(&b, "%-5s  %-*s  (%s, %dms)%s\n", strings.ToUpper(string(r.Status)), width, r.Rule, r.Module, r.DurationMS, stateNote(r))
		for _, f := range r.Failures {
			fmt.Fprintf(&b, "       %s\n", f.Message)
		}
		if r.Error != nil {
			fmt.Fprintf(&b, "       %s\n", *r.Error)
		}
	}
	if len(shown) > 0 {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%d rules: %d passed, %d failed, %d errors\n", summary.Total, summary.Passed, summary.Failed, summary.Errors)
	_, err := io.WriteString(w, b.String())
	return err
}

// stateNote explains the confirmed state when it is not obvious from the
// status: a transition happened, or a threshold has not been reached yet.
func stateNote(r result.Result) string {
	switch {
	case r.Transition != nil:
		return fmt.Sprintf("  [transition: %s]", *r.Transition)
	case !r.OK() && r.State == "pass":
		return fmt.Sprintf("  [state: pass, %d/%d]", r.Consecutive, r.FailAfter)
	case r.OK() && r.State == "fail":
		return fmt.Sprintf("  [state: fail, %d/%d]", r.Consecutive, r.PassAfter)
	}
	return ""
}
