// Package logging sets up onecheck's logger: everything at the configured
// level goes to the log file, warnings and errors are repeated on stderr.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// ParseLevel converts a level name into a slog level.
func ParseLevel(name string) (slog.Level, error) {
	switch name {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("unknown log level %q (expected debug, info, warn or error)", name)
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// Setup returns a logger writing to file (format "text" or "json") at the
// given level and to stderr at warn and above. When the file cannot be
// opened it still returns a working logger, which then sends everything at
// the level to stderr, together with an error describing the problem.
func Setup(file, level, format string, stderr io.Writer) (*slog.Logger, io.Closer, error) {
	lvl, err := ParseLevel(level)
	if err != nil {
		return nil, nil, err
	}
	f, err := openLog(file)
	if err != nil {
		h := slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: lvl})
		return slog.New(h), nopCloser{}, fmt.Errorf("cannot open log file, logging to stderr only: %w", err)
	}
	var fileHandler slog.Handler
	if format == "json" {
		fileHandler = slog.NewJSONHandler(f, &slog.HandlerOptions{Level: lvl})
	} else {
		fileHandler = slog.NewTextHandler(f, &slog.HandlerOptions{Level: lvl})
	}
	stderrHandler := slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelWarn})
	return slog.New(fanout{fileHandler, stderrHandler}), f, nil
}

func openLog(file string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// fanout sends each record to every handler that accepts its level.
type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	var first error
	for _, h := range f {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		if err := h.Handle(ctx, r.Clone()); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (f fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(attrs)
	}
	return out
}

func (f fanout) WithGroup(name string) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(name)
	}
	return out
}
