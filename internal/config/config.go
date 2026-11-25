// Package config finds and loads onecheck configuration files.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// systemDir is the default config directory for the whole host.
var systemDir = "/etc/onecheck"

// SetSystemDir replaces the host config directory and returns a function
// that restores it. Tests use it so that the machine's own configs never
// leak into them.
func SetSystemDir(dir string) (restore func()) {
	old := systemDir
	systemDir = dir
	return func() { systemDir = old }
}

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

// Where the configs were found, as reported in Target.Source.
const (
	SourceFlag    = "--config"
	SourceEnv     = "ONECHECK_CONFIG"
	SourceDefault = "default location"
)

// Target is what onecheck was pointed at: one config file or a directory of
// config files.
type Target struct {
	Path   string   // the file or directory that was given or found
	Source string   // SourceFlag, SourceEnv or SourceDefault
	Dir    bool     // Path is a directory
	Files  []string // the config files to load, in order
}

// Discover finds the configs to use. The path comes from --config, then
// $ONECHECK_CONFIG; it may be a file or a directory. Without both, the first
// of /etc/onecheck and $HOME/.config/onecheck that holds a config file is
// used.
func Discover(flag string) (*Target, error) {
	path, source := flag, SourceFlag
	if path == "" {
		path, source = os.Getenv("ONECHECK_CONFIG"), SourceEnv
	}
	if path != "" {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("config file %s: %w", path, err)
		}
		if !info.IsDir() {
			return &Target{Path: path, Source: source, Files: []string{path}}, nil
		}
		files, err := ConfigFiles(path)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("no config files (*.yaml, *.yml) in %s", path)
		}
		return &Target{Path: path, Source: source, Dir: true, Files: files}, nil
	}
	candidates := []string{systemDir}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "onecheck"))
	}
	for _, dir := range candidates {
		if files, err := ConfigFiles(dir); err == nil && len(files) > 0 {
			return &Target{Path: dir, Source: SourceDefault, Dir: true, Files: files}, nil
		}
	}
	msg := "no config files (*.yaml, *.yml) found; looked in:"
	for _, dir := range candidates {
		msg += "\n  " + dir
	}
	return nil, fmt.Errorf("%s\nuse --config <file or directory> or set ONECHECK_CONFIG", msg)
}

// ConfigFiles lists the config files of a directory: regular files (symlinks
// followed) named *.yaml or *.yml and not starting with ".", sorted by name.
// Subdirectories are not entered.
func ConfigFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("config directory %s: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || (!strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml")) {
			continue
		}
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, path)
	}
	return files, nil
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
	c.Defaults = Defaults{Timeout: Duration(30 * time.Second), FailAfter: 1, PassAfter: 1, OutputTail: 10 << 10, EventParams: true}
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
