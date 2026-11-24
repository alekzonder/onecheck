package main

import (
	"strings"
	"time"
)

// timeLayout is how systemctl prints timestamps under LC_ALL=C TZ=UTC.
const timeLayout = "Mon 2006-01-02 15:04:05 MST"

// parseShow reads "systemctl show" output: one Key=Value per line. Values
// may contain "=", so a line is split on the first one only.
func parseShow(text string) map[string]string {
	p := map[string]string{}
	for line := range strings.SplitSeq(text, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || k == "" {
			continue
		}
		p[k] = v
	}
	return p
}

// parseTime parses a systemctl timestamp. Empty, "n/a" and "0" mean that
// systemd has no such time.
func parseTime(v string) (time.Time, bool) {
	switch v {
	case "", "n/a", "0":
		return time.Time{}, false
	}
	t, err := time.Parse(timeLayout, v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
