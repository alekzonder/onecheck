package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alekzonder/onecheck/internal/config"
)

const pathsLong = `Show the full path of everything onecheck reads and writes for this config.

  config    the config file, and how it was found
  name      the config's name; it is part of the log, state and results paths
  include   every include file that was read
  log       the log file
  state     the confirmed state of each rule, kept between runs
  results   the latest result of each rule
  events    the event queue: one file per pass/fail transition or reminder
  modules   where modules are looked for
  binary    the onecheck executable itself

A path that does not exist yet is marked "(missing)"; onecheck creates the
log, state, results and events paths on the first run. Nothing is created or
changed by this command, and modules do not have to be installed.

If the config is found but cannot be loaded, its path is still printed and
the command exits with 2.`

// pathEntry is one path in the JSON output of "onecheck paths".
type pathEntry struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Source string `json:"source,omitempty"`
}

// pathsReport is the JSON output of "onecheck paths".
type pathsReport struct {
	Name     string      `json:"name"`
	Config   pathEntry   `json:"config"`
	Includes []pathEntry `json:"includes"`
	Log      *pathEntry  `json:"log,omitempty"`
	State    *pathEntry  `json:"state,omitempty"`
	Results  *pathEntry  `json:"results,omitempty"`
	Events   *pathEntry  `json:"events,omitempty"`
	Modules  *pathEntry  `json:"modules,omitempty"`
	Binary   pathEntry   `json:"binary"`
}

func entry(path string) pathEntry {
	_, err := os.Stat(path)
	return pathEntry{Path: path, Exists: err == nil}
}

func entryPtr(path string) *pathEntry {
	e := entry(path)
	return &e
}

// binaryPath is the real location of the running executable.
func binaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return os.Args[0]
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		return real
	}
	return exe
}

func (a *app) pathsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:     "paths",
		Short:   "Show the config, log, state, results, events and modules paths",
		Long:    pathsLong,
		Example: "  onecheck paths\n  onecheck paths -c ./config.yaml --json",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return a.paths(asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the paths as JSON")
	return cmd
}

func (a *app) paths(asJSON bool) error {
	path, source, err := config.Locate(a.configPath)
	if err != nil {
		return err
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	report := pathsReport{Config: entry(path), Includes: []pathEntry{}, Binary: entry(binaryPath())}
	report.Config.Source = source

	cfg, loadErr := config.Load(path)
	if loadErr == nil {
		report.Name = cfg.Name
		for _, inc := range cfg.Includes {
			report.Includes = append(report.Includes, entry(inc))
		}
		report.Log = entryPtr(cfg.LogFile())
		report.State = entryPtr(cfg.StateDir())
		report.Results = entryPtr(cfg.ResultsDir())
		report.Events = entryPtr(cfg.EventsDir())
		report.Modules = entryPtr(cfg.Paths.Modules)
	}

	if asJSON {
		enc := json.NewEncoder(a.stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
		return loadErr
	}
	a.printPaths(report)
	return loadErr
}

func (a *app) printPaths(r pathsReport) {
	row := func(label string, e *pathEntry) {
		if e == nil {
			return
		}
		note := ""
		switch {
		case !e.Exists:
			note = "  (missing)"
		case e.Source != "":
			note = "  (from " + e.Source + ")"
		}
		fmt.Fprintf(a.stdout, "%-9s %s%s\n", label, e.Path, note)
	}
	row("config", &r.Config)
	if r.Name != "" {
		fmt.Fprintf(a.stdout, "%-9s %s\n", "name", r.Name)
	}
	for i := range r.Includes {
		row("include", &r.Includes[i])
	}
	row("log", r.Log)
	row("state", r.State)
	row("results", r.Results)
	row("events", r.Events)
	row("modules", r.Modules)
	row("binary", &r.Binary)
}
