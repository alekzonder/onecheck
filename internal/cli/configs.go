package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alekzonder/onecheck/internal/config"
	"github.com/alekzonder/onecheck/internal/logging"
	"github.com/alekzonder/onecheck/internal/runner"
)

// loadedConfig is one config file of the target: the loaded config, or why
// it could not be loaded.
type loadedConfig struct {
	path string
	cfg  *config.Config
	err  error
}

// loadConfigs finds and loads the configs and applies --log-level to each.
// For a single config file a load error is returned as is; in a directory
// every file gets an entry, broken or not, so that one broken config does
// not hide the others.
func (a *app) loadConfigs() (*config.Target, []loadedConfig, error) {
	if a.logLevel != "" {
		if _, err := logging.ParseLevel(a.logLevel); err != nil {
			return nil, nil, err
		}
	}
	target, err := config.Discover(a.configPath)
	if err != nil {
		return nil, nil, err
	}
	list := make([]loadedConfig, len(target.Files))
	for i, file := range target.Files {
		if abs, err := filepath.Abs(file); err == nil {
			file = abs
		}
		cfg, err := config.Load(file)
		if err == nil && a.logLevel != "" {
			cfg.Log.Level = a.logLevel
		}
		list[i] = loadedConfig{path: file, cfg: cfg, err: err}
	}
	if !target.Dir && list[0].err != nil {
		return nil, nil, list[0].err
	}
	markDuplicateNames(list)
	return target, list, nil
}

// markDuplicateNames turns configs sharing a name into errors: they would
// share state, results and logs.
func markDuplicateNames(list []loadedConfig) {
	first := map[string]int{}
	for i, l := range list {
		if l.err != nil {
			continue
		}
		j, seen := first[l.cfg.Name]
		if !seen {
			first[l.cfg.Name] = i
			continue
		}
		list[i].err = fmt.Errorf("name %q is also used by %s", l.cfg.Name, list[j].path)
		if list[j].cfg != nil {
			list[j].err = fmt.Errorf("name %q is also used by %s", l.cfg.Name, l.path)
			list[j].cfg = nil
		}
		list[i].cfg = nil
	}
}

// configError is the error of a config as one line that names its file.
func configError(path string, err error) string {
	msg := err.Error()
	if !strings.Contains(msg, path) {
		msg = path + ": " + msg
	}
	return msg
}

// checkRuleNames makes sure every --rule names a rule of some config.
func checkRuleNames(list []loadedConfig, names []string, where string) error {
	known := map[string]bool{}
	for _, l := range list {
		if l.cfg == nil {
			continue
		}
		for _, r := range l.cfg.Rules {
			known[r.Name] = true
		}
	}
	for _, name := range names {
		if !known[name] {
			return fmt.Errorf("no rule named %q in %s", name, where)
		}
	}
	return nil
}

// narrowOptions keeps the --rule names this config has. ok is false when the
// run was filtered and nothing of the filter applies to this config: an
// empty filter would run every rule.
func narrowOptions(cfg *config.Config, opts runner.Options) (runner.Options, bool) {
	if len(opts.Rules) == 0 {
		return opts, true
	}
	var rules []string
	for _, name := range opts.Rules {
		for _, r := range cfg.Rules {
			if r.Name == name {
				rules = append(rules, name)
				break
			}
		}
	}
	out := runner.Options{Rules: rules, Tags: opts.Tags}
	return out, len(rules) > 0 || len(opts.Tags) > 0
}
