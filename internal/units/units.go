// Package units parses human-friendly sizes ("10MB") and durations ("30d").
package units

import (
	"fmt"
	"strconv"
	"strings"
)

var byteUnits = map[string]float64{
	"":   1,
	"b":  1,
	"k":  1 << 10,
	"kb": 1 << 10,
	"m":  1 << 20,
	"mb": 1 << 20,
	"g":  1 << 30,
	"gb": 1 << 30,
	"t":  1 << 40,
	"tb": 1 << 40,
}

var durationUnits = map[string]float64{
	"ns": 1e-9,
	"us": 1e-6,
	"µs": 1e-6,
	"ms": 1e-3,
	"s":  1,
	"m":  60,
	"h":  3600,
	"d":  86400,
	"w":  604800,
}

// ParseBytes converts a size such as "10MB", "1.5KB" or "10k" into bytes.
// Units are 1024-based and case-insensitive; a bare number means bytes.
func ParseBytes(s string) (float64, error) {
	num, unit := splitNumber(strings.TrimSpace(s))
	mult, ok := byteUnits[strings.ToLower(strings.TrimSpace(unit))]
	if num == "" || !ok {
		return 0, fmt.Errorf("invalid size %q (expected a number with an optional unit B, K/KB, M/MB, G/GB or T/TB)", s)
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return v * mult, nil
}

// ParseDuration converts a duration such as "1h30m", "30d" or "2w" into
// seconds. A bare number means seconds.
func ParseDuration(s string) (float64, error) {
	rest := strings.TrimSpace(s)
	if v, err := strconv.ParseFloat(rest, 64); err == nil && v >= 0 {
		return v, nil
	}
	if rest == "" {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	total := 0.0
	for rest != "" {
		num, tail := splitNumber(rest)
		unit, next := splitUnit(tail)
		mult, ok := durationUnits[unit]
		if num == "" || !ok {
			return 0, fmt.Errorf("invalid duration %q (expected e.g. 30s, 5m, 1h30m, 7d, 2w)", s)
		}
		v, err := strconv.ParseFloat(num, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		total += v * mult
		rest = next
	}
	return total, nil
}

// splitNumber splits s into its leading decimal number and the remainder.
func splitNumber(s string) (num, rest string) {
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	return s[:i], s[i:]
}

// splitUnit splits s into its leading non-numeric unit and the remainder.
func splitUnit(s string) (unit, rest string) {
	i := 0
	for i < len(s) && (s[i] < '0' || s[i] > '9') && s[i] != '.' {
		i++
	}
	return strings.TrimSpace(s[:i]), s[i:]
}
