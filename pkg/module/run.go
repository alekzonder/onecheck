package module

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

// CollectFunc gathers field values for one check. Returning an error means
// the data could not be collected at all (no network, no permission); the
// absence of the checked object is a value, not an error.
type CollectFunc func(ctx context.Context, req Request) (map[string]any, error)

// Module is a complete check module: its description and its collector.
type Module struct {
	Description Description
	Collect     CollectFunc
}

// Main runs the module with the process arguments and standard streams and
// exits. Call it from func main.
func Main(m Module) {
	os.Exit(Run(m, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// Run executes one module invocation and returns the process exit code.
func Run(m Module, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "describe":
		d := m.Description
		d.Protocol = ProtocolVersion
		return writeJSON(stdout, stderr, d)
	case "check":
		return runCheck(m, stdin, stdout, stderr)
	default:
		name := m.Description.Name
		fmt.Fprintf(stderr, "onecheck-%s is a onecheck module and is normally started by onecheck.\n\n"+
			"usage:\n  onecheck-%s describe   print module description as JSON\n"+
			"  onecheck-%s check      read a check request from stdin, print field values as JSON\n", name, name, name)
		return 2
	}
}

func runCheck(m Module, stdin io.Reader, stdout, stderr io.Writer) int {
	var req Request
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		fmt.Fprintf(stderr, "invalid request: %v\n", err)
		return 1
	}
	if req.Protocol != ProtocolVersion {
		fmt.Fprintf(stderr, "unsupported protocol %d (module speaks protocol %d)\n", req.Protocol, ProtocolVersion)
		return 1
	}
	ctx := context.Background()
	if req.TimeoutMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutMS)*time.Millisecond)
		defer cancel()
	}
	resp := Response{Fields: map[string]any{}, Truncated: []string{}}
	fields, err := m.Collect(ctx, req)
	if err != nil {
		resp.Error = &Error{Message: err.Error()}
		return writeJSON(stdout, stderr, resp)
	}
	for name, v := range fields {
		if s, ok := v.(string); ok && req.MaxFieldSize > 0 && int64(len(s)) > req.MaxFieldSize {
			v = s[:req.MaxFieldSize]
			resp.Truncated = append(resp.Truncated, name)
		}
		resp.Fields[name] = v
	}
	sort.Strings(resp.Truncated)
	return writeJSON(stdout, stderr, resp)
}

func writeJSON(stdout, stderr io.Writer, v any) int {
	if err := json.NewEncoder(stdout).Encode(v); err != nil {
		fmt.Fprintf(stderr, "cannot write response: %v\n", err)
		return 1
	}
	return 0
}
