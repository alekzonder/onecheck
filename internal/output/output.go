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

// Config is what one config contributed to a run: its results, or why it
// could not run.
type Config struct {
	Name    string // the config's name; empty if it could not be loaded
	Path    string // the config file
	Error   string // why the config could not run; empty if it ran
	Results []result.Result
}

// configJSON is an element of "configs" in the JSON output.
type configJSON struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Error string `json:"error,omitempty"`
}

// ExitCode is 2 when a config could not run, else 1 when a result did not
// pass, else 0. It reflects the raw status of this run, not the confirmed
// state.
func ExitCode(configs []Config) int {
	code := 0
	for _, c := range configs {
		if c.Error != "" {
			return 2
		}
		for _, r := range c.Results {
			if !r.OK() {
				code = 1
			}
		}
	}
	return code
}

// Write renders the results in the given format ("text" or "json"). With
// onlyFailed, passing rules are left out of the list but not of the summary.
// grouped prints the text output per config, for a directory of configs.
func Write(w io.Writer, format string, onlyFailed, grouped bool, configs []Config) error {
	all := []result.Result{}
	shown := []result.Result{}
	meta := make([]configJSON, len(configs))
	for i, c := range configs {
		meta[i] = configJSON{Name: c.Name, Path: c.Path, Error: c.Error}
		all = append(all, c.Results...)
		shown = append(shown, visible(c.Results, onlyFailed)...)
	}
	summary := Summarize(all)
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Summary Summary         `json:"summary"`
			Configs []configJSON    `json:"configs"`
			Results []result.Result `json:"results"`
		}{summary, meta, shown})
	case "text":
		var b strings.Builder
		if grouped {
			writeGroups(&b, configs, onlyFailed)
		} else {
			writeRules(&b, shown, ruleWidth(shown))
		}
		if len(shown) > 0 || b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%d rules: %d passed, %d failed, %d errors", summary.Total, summary.Passed, summary.Failed, summary.Errors)
		if broken := brokenConfigs(configs); grouped && broken > 0 {
			fmt.Fprintf(&b, "; %d of %d configs could not run", broken, len(configs))
		}
		b.WriteString("\n")
		_, err := io.WriteString(w, b.String())
		return err
	}
	return fmt.Errorf("unknown output format %q (expected text or json)", format)
}

func visible(results []result.Result, onlyFailed bool) []result.Result {
	out := []result.Result{}
	for _, r := range results {
		if !onlyFailed || !r.OK() {
			out = append(out, r)
		}
	}
	return out
}

func brokenConfigs(configs []Config) int {
	n := 0
	for _, c := range configs {
		if c.Error != "" {
			n++
		}
	}
	return n
}

func ruleWidth(results []result.Result) int {
	width := 0
	for _, r := range results {
		width = max(width, len(r.Rule))
	}
	return width
}

// writeGroups prints a header and the rules of each config. A config with
// nothing to show (only passing rules under onlyFailed) is left out.
func writeGroups(b *strings.Builder, configs []Config, onlyFailed bool) {
	width := 0
	for _, c := range configs {
		width = max(width, ruleWidth(c.Results))
	}
	first := true
	for _, c := range configs {
		shown := visible(c.Results, onlyFailed)
		if c.Error == "" && len(shown) == 0 && onlyFailed {
			continue
		}
		if !first {
			b.WriteString("\n")
		}
		first = false
		if c.Name != "" {
			fmt.Fprintf(b, "== %s  %s\n", c.Name, c.Path)
		} else {
			fmt.Fprintf(b, "== %s\n", c.Path)
		}
		if c.Error != "" {
			fmt.Fprintf(b, "ERROR  config: %s\n", c.Error)
		}
		writeRules(b, shown, width)
	}
}

func writeRules(b *strings.Builder, shown []result.Result, width int) {
	for _, r := range shown {
		fmt.Fprintf(b, "%-5s  %-*s  (%s, %dms)%s\n", strings.ToUpper(string(r.Status)), width, r.Rule, r.Module, r.DurationMS, stateNote(r))
		for _, f := range r.Failures {
			fmt.Fprintf(b, "       %s\n", f.Message)
		}
		if r.Error != nil {
			fmt.Fprintf(b, "       %s\n", *r.Error)
		}
	}
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
