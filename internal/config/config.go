// Package config finds and loads onecheck configuration files.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const systemConfig = "/etc/onecheck/config.yaml"

// NameRE is the pattern for config and rule names. Names become file names.
var NameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

// ResultsDir is the directory holding this config's result files.
func (c *Config) ResultsDir() string { return filepath.Join(c.Paths.Results, c.Name) }

// StateDir is the directory holding this config's state files and lock.
func (c *Config) StateDir() string { return filepath.Join(c.Paths.State, c.Name) }

// EventsDir is the event queue directory shared by all configs.
func (c *Config) EventsDir() string { return c.Paths.Events }

// LogFile is this config's log file.
func (c *Config) LogFile() string { return filepath.Join(c.Log.Dir, c.Name, "onecheck.log") }

// Find returns the config file to use: the explicit path, $ONECHECK_CONFIG,
// /etc/onecheck/config.yaml, then $HOME/.config/onecheck/config.yaml.
func Find(flag string) (string, error) {
	if flag == "" {
		flag = os.Getenv("ONECHECK_CONFIG")
	}
	if flag != "" {
		if _, err := os.Stat(flag); err != nil {
			return "", fmt.Errorf("config file %s: %w", flag, err)
		}
		return flag, nil
	}
	candidates := []string{systemConfig}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "onecheck", "config.yaml"))
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	msg := "no config file found; looked in:"
	for _, path := range candidates {
		msg += "\n  " + path
	}
	return "", fmt.Errorf("%s\nuse --config <path> or set ONECHECK_CONFIG", msg)
}

// defaultConfig returns a config with every default applied. Root gets
// system directories, other users directories under $HOME.
func defaultConfig() *Config {
	c := &Config{}
	c.Log.Level = "info"
	c.Log.Format = "text"
	c.Log.Rotate = Rotate{MaxSize: 10 << 20, Keep: 5, MaxAge: Duration(30 * 24 * time.Hour), Compress: true}
	c.Events.TTL = Duration(24 * time.Hour)
	c.Run = Run{Parallel: 4, MaxFieldSize: 10 << 20}
	c.Defaults = Defaults{Timeout: Duration(30 * time.Second), FailAfter: 1, PassAfter: 1, OutputTail: 10 << 10}
	c.Modules.Repos = []string{"alekzonder/onecheck"}

	home, err := os.UserHomeDir()
	if os.Geteuid() == 0 || err != nil {
		c.Log.Dir = "/var/log/onecheck"
		c.Paths = Paths{
			Modules: "/usr/lib/onecheck/modules",
			Results: "/var/lib/onecheck/results",
			Events:  "/var/lib/onecheck/events",
			State:   "/var/lib/onecheck/state",
		}
		return c
	}
	state := filepath.Join(home, ".local", "state", "onecheck")
	c.Log.Dir = filepath.Join(state, "log")
	c.Paths = Paths{
		Modules: filepath.Join(home, ".local", "lib", "onecheck", "modules"),
		Results: filepath.Join(state, "results"),
		Events:  filepath.Join(state, "events"),
		State:   filepath.Join(state, "state"),
	}
	return c
}
