package main

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/alekzonder/onecheck/internal/glob"
)

// unit is one element of the units field.
type unit struct {
	Name        string `json:"name"`
	LoadState   string `json:"load_state"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	Description string `json:"description"`
}

// stateName is a systemd load, active or sub state: failed, not-found, ...
var stateName = regexp.MustCompile(`^[a-z][a-z-]*$`)

// collectList reports the units in params.state, without those a pattern
// of params.exclude matches.
func collectList(ctx context.Context, p params) (map[string]any, error) {
	exclude := make([]*regexp.Regexp, 0, len(p.Exclude))
	for _, pattern := range p.Exclude {
		re, err := glob.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid params: exclude: %w", err)
		}
		exclude = append(exclude, re)
	}
	out, err := systemctl(ctx, p.User, "list-units", "--all", "--plain", "--no-legend", "--no-pager", "--state="+p.State)
	if err != nil {
		return nil, fmt.Errorf("systemctl list-units: %w", err)
	}
	units, names := []unit{}, []string{}
	for _, u := range parseUnits(out) {
		if !matchesAny(exclude, u.Name) {
			units = append(units, u)
		}
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })
	for _, u := range units {
		names = append(names, u.Name)
	}
	return map[string]any{"units": units, "names": names, "count": len(units)}, nil
}

func matchesAny(patterns []*regexp.Regexp, name string) bool {
	for _, re := range patterns {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

// parseUnits reads "systemctl list-units --plain --no-legend": one unit per
// line as "UNIT LOAD ACTIVE SUB DESCRIPTION", the description being the rest
// of the line.
func parseUnits(text string) []unit {
	var units []unit
	for line := range strings.SplitSeq(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		u := unit{Name: f[0], LoadState: f[1], ActiveState: f[2], SubState: f[3]}
		rest := strings.TrimSpace(line)
		for _, word := range f[:4] {
			rest = strings.TrimSpace(strings.TrimPrefix(rest, word))
		}
		u.Description = rest
		units = append(units, u)
	}
	return units
}
