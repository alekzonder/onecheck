package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// show runs "systemctl show" for one unit and returns its properties.
func show(ctx context.Context, user bool, unit string, props []string) (map[string]string, error) {
	// "--" keeps a unit name that starts with "-" from being read as an option.
	out, err := systemctl(ctx, user, "show", "-p", strings.Join(props, ","), "--", unit)
	if err != nil {
		return nil, fmt.Errorf("systemctl show %s: %w", unit, err)
	}
	if strings.Contains(out, "\n\n") {
		return nil, fmt.Errorf("systemctl show %s printed several units", unit)
	}
	return parseShow(out), nil
}

// systemctl runs systemctl, for the user's service manager if user is set,
// and returns its trimmed output; on failure the error is what systemctl
// printed. Timestamps are printed in UTC with C locale so parseTime can read
// them.
func systemctl(ctx context.Context, user bool, args ...string) (string, error) {
	bin, err := exec.LookPath("systemctl")
	if err != nil {
		return "", errors.New("systemd is not available: systemctl not found in PATH")
	}
	if user {
		args = append([]string{"--user"}, args...)
	}
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // fixed program, every value passed as one argument
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	if user && os.Getenv("XDG_RUNTIME_DIR") == "" {
		cmd.Env = append(cmd.Env, "XDG_RUNTIME_DIR=/run/user/"+strconv.Itoa(os.Getuid()))
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}
