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
// Timestamps are printed in UTC with C locale so parseTime can read them.
func show(ctx context.Context, user bool, unit string, props []string) (map[string]string, error) {
	bin, err := exec.LookPath("systemctl")
	if err != nil {
		return nil, errors.New("systemd is not available: systemctl not found in PATH")
	}
	var args []string
	if user {
		args = append(args, "--user")
	}
	args = append(args, "show", unit, "-p", strings.Join(props, ","))
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // fixed program, unit passed as one argument
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	if user && os.Getenv("XDG_RUNTIME_DIR") == "" {
		cmd.Env = append(cmd.Env, "XDG_RUNTIME_DIR=/run/user/"+strconv.Itoa(os.Getuid()))
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("systemctl show %s: %s", unit, msg)
		}
		return nil, fmt.Errorf("systemctl show %s: %w", unit, err)
	}
	return parseShow(stdout.String()), nil
}
