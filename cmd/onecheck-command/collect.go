package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/alekzonder/onecheck/pkg/module"
)

type params struct {
	Run   string            `json:"run"`
	Argv  []string          `json:"argv"`
	Env   map[string]string `json:"env"`
	Cwd   string            `json:"cwd"`
	Stdin string            `json:"stdin"`
}

// capture keeps the first limit+1 bytes written to it and drops the rest
// without ever blocking or failing the writer.
type capture struct {
	buf   bytes.Buffer
	limit int64
}

func (c *capture) Write(p []byte) (int, error) {
	if c.limit <= 0 {
		return c.buf.Write(p)
	}
	if room := c.limit + 1 - int64(c.buf.Len()); room > 0 {
		c.buf.Write(p[:min(int64(len(p)), room)])
	}
	return len(p), nil
}

func collect(ctx context.Context, req module.Request) (map[string]any, error) {
	var p params
	if err := module.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	if (p.Run == "") == (len(p.Argv) == 0) {
		return nil, errors.New("invalid params: exactly one of run, argv is required")
	}
	var cmd *exec.Cmd
	if p.Run != "" {
		cmd = exec.CommandContext(ctx, "sh", "-c", p.Run)
	} else {
		cmd = exec.CommandContext(ctx, p.Argv[0], p.Argv[1:]...)
	}
	cmd.Dir = p.Cwd
	cmd.Env = os.Environ()
	for name, value := range p.Env {
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	cmd.Stdin = strings.NewReader(p.Stdin)
	stdout, stderr := &capture{limit: req.MaxFieldSize}, &capture{limit: req.MaxFieldSize}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	// The command stays in the module's process group on purpose: onecheck
	// kills that whole group on timeout or interruption, which takes the
	// command and everything it started with it.
	cmd.WaitDelay = time.Second

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start).Seconds()
	if ctx.Err() != nil {
		// what the command printed before it was killed helps to see where it hung
		partial := map[string]any{"stdout": stdout.buf.String(), "stderr": stderr.buf.String()}
		return partial, fmt.Errorf("command timed out after %.1fs", duration)
	}
	exitCode := 0
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		exitCode = exitErr.ExitCode() // a non-zero exit is a value to check, not an error
	case err != nil:
		return nil, fmt.Errorf("cannot run command: %w", err)
	}
	return map[string]any{
		"exit_code": exitCode,
		"stdout":    stdout.buf.String(),
		"stderr":    stderr.buf.String(),
		"duration":  duration,
	}, nil
}
