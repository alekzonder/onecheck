package cli

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/alekzonder/onecheck/internal/config"
	"github.com/alekzonder/onecheck/internal/logging"
)

// loadConfig finds and loads the config and applies the --log-level flag.
func (a *app) loadConfig() (*config.Config, error) {
	path, err := config.Find(a.configPath)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	if a.logLevel != "" {
		if _, err := logging.ParseLevel(a.logLevel); err != nil {
			return nil, err
		}
		cfg.Log.Level = a.logLevel
	}
	return cfg, nil
}

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
