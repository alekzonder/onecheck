package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"

	"github.com/alekzonder/onecheck/pkg/module"
)

// killGrace is how long a module may overrun its timeout to report the
// timeout itself before onecheck kills it.
const killGrace = 500 * time.Millisecond

var errTooLarge = errors.New("output too large")

// limitedBuffer keeps at most max bytes and remembers an overflow.
type limitedBuffer struct {
	buf      bytes.Buffer // not embedded: its ReadFrom would bypass the limit
	max      int64
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if int64(b.buf.Len()+len(p)) > b.max {
		b.overflow = true
		return 0, errTooLarge
	}
	return b.buf.Write(p)
}

// Check runs one check through the module and returns its response. An error
// means the module itself misbehaved: timeout, crash, invalid output.
func Check(ctx context.Context, info *Info, req module.Request, timeout time.Duration, stderr io.Writer) (*module.Response, error) {
	req.Protocol = info.Protocol
	req.TimeoutMS = timeout.Milliseconds()
	input, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("cannot encode request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout+killGrace)
	defer cancel()

	// JSON escaping can inflate a value up to six times.
	stdout := &limitedBuffer{max: 6*req.MaxFieldSize*int64(len(req.Fields)+1) + 1<<20}
	cmd := exec.CommandContext(ctx, info.Path, "check")
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// Own process group, so that a timeout also kills the module's children.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second

	err = cmd.Run()
	switch {
	case ctx.Err() != nil:
		return nil, fmt.Errorf("module %q timed out after %s", info.Name, timeout)
	case stdout.overflow:
		return nil, fmt.Errorf("module %q: output too large (limit %d bytes)", info.Name, stdout.max)
	case err != nil:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("module %q failed with exit code %d", info.Name, exitErr.ExitCode())
		}
		return nil, fmt.Errorf("module %q could not be started: %w", info.Name, err)
	}
	var resp module.Response
	if err := json.Unmarshal(stdout.buf.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("module %q returned invalid JSON: %w", info.Name, err)
	}
	return &resp, nil
}
