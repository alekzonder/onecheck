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

With a directory of configs (--config <dir>, or no --config at all) a block
is printed for every config in it, then the binary once; --json prints
{"configs": [...], "binary": {...}}.

If a config is found but cannot be loaded, its path is still printed and
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
	Binary   *pathEntry  `json:"binary,omitempty"`
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
	target, err := config.Discover(a.configPath)
	if err != nil {
		return err
	}
	reports := make([]pathsReport, len(target.Files))
	var loadErrs []error
	for i, file := range target.Files {
		var loadErr error
		reports[i], loadErr = configPaths(file, target.Source)
		if loadErr != nil {
			loadErrs = append(loadErrs, loadErr)
		}
	}
	binary := entry(binaryPath())
	if !target.Dir {
		reports[0].Binary = &binary
		if err := a.writePaths(asJSON, reports[0]); err != nil {
			return err
		}
		if len(loadErrs) > 0 {
			return loadErrs[0]
		}
		return nil
	}
	if err := a.writeDirPaths(asJSON, reports, binary); err != nil {
		return err
	}
	for _, e := range loadErrs {
		fmt.Fprintf(a.stderr, "onecheck: %v\n", e)
	}
	if len(loadErrs) > 0 {
		return fmt.Errorf("%d of %d configs in %s could not be loaded", len(loadErrs), len(reports), target.Path)
	}
	return nil
}

// configPaths is the report of one config file. A config that cannot be
// loaded still reports its own path.
func configPaths(path, source string) (pathsReport, error) {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	report := pathsReport{Config: entry(path), Includes: []pathEntry{}}
	report.Config.Source = source
	cfg, err := config.Load(path)
	if err != nil {
		return report, err
	}
	report.Name = cfg.Name
	for _, inc := range cfg.Includes {
		report.Includes = append(report.Includes, entry(inc))
	}
	report.Log = entryPtr(cfg.LogFile())
	report.State = entryPtr(cfg.StateDir())
	report.Results = entryPtr(cfg.ResultsDir())
	report.Events = entryPtr(cfg.EventsDir())
	report.Modules = entryPtr(cfg.Paths.Modules)
	return report, nil
}

func (a *app) writePaths(asJSON bool, report pathsReport) error {
	if asJSON {
		enc := json.NewEncoder(a.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	a.printPaths(report)
	return nil
}

// writeDirPaths prints one block per config and the binary once.
func (a *app) writeDirPaths(asJSON bool, reports []pathsReport, binary pathEntry) error {
	if asJSON {
		enc := json.NewEncoder(a.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Configs []pathsReport `json:"configs"`
			Binary  pathEntry     `json:"binary"`
		}{reports, binary})
	}
	for _, r := range reports {
		a.printPaths(r)
		fmt.Fprintln(a.stdout)
	}
	a.printPaths(pathsReport{Binary: &binary})
	return nil
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
	if r.Config.Path != "" {
		row("config", &r.Config)
	}
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
	row("binary", r.Binary)
}
