package cli

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/alekzonder/onecheck/internal/config"
	"github.com/alekzonder/onecheck/internal/logging"
)

// logger opens the config's log. A log file that cannot be opened is only a
// warning: checks are more important than their log. A level given with
// --log-level is also what stderr shows: whoever typed it is watching.
func (a *app) logger(cfg *config.Config) (*slog.Logger, io.Closer, error) {
	log, closer, err := logging.Setup(cfg.LogFile(), cfg.Log.Level, cfg.Log.Format, a.stderr, a.logLevel)
	if log == nil {
		return nil, nil, err
	}
	if err != nil {
		fmt.Fprintf(a.stderr, "onecheck: warning: %v\n", err)
	}
	return log, closer, nil
}
