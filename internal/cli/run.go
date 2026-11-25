package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alekzonder/onecheck/internal/output"
	"github.com/alekzonder/onecheck/internal/runner"
	"github.com/alekzonder/onecheck/internal/version"
)

type runOptions struct {
	output     string
	onlyFailed bool
	rules      []string
	tags       []string
}

const runLong = `Run the rules of the config and report the outcome of each.

For every rule onecheck starts the rule's module, evaluates the "check"
conditions on what the module returned, updates the rule's state, and writes:

  <paths.results>/<config name>/<rule>.json   the latest result of the rule
  <paths.events>/<id>.json                    an event, only when the rule
                                              changed between pass and fail

A rule that cannot be checked (module missing, invalid params, timeout) gets
the status "error"; the other rules still run.

Without --rule and --tag every enabled rule runs. With them, a rule runs if
it is named by --rule or carries one of the --tag tags.

With a directory of configs, each config runs in turn with its own lock,
log, state and results, and the output groups the rules by config. A config
that cannot be loaded or run is reported and the others still run; the exit
code is then 2. A --rule must exist in at least one of the configs.`

const runExample = `  onecheck run
  onecheck run -c ./config.yaml --only-failed
  onecheck run -c /etc/onecheck -o json
  onecheck run -o json | jq '.results[] | select(.status != "pass")'
  onecheck run --rule api-health --rule disk-space
  onecheck run --tag web --log-level debug`

func addRunFlags(cmd *cobra.Command, o *runOptions) {
	cmd.Flags().StringVarP(&o.output, "output", "o", "text", "output format: text or json")
	cmd.Flags().BoolVar(&o.onlyFailed, "only-failed", false, "print only rules that failed or could not be checked")
	cmd.Flags().StringArrayVar(&o.rules, "rule", nil, "run only this rule (repeatable)")
	cmd.Flags().StringArrayVar(&o.tags, "tag", nil, "run only rules with this tag (repeatable)")
}

func (a *app) runCmd() *cobra.Command {
	o := &runOptions{}
	cmd := &cobra.Command{
		Use:     "run",
		Short:   "Run the checks (default command)",
		Long:    runLong,
		Example: runExample,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd.Context(), o)
		},
	}
	addRunFlags(cmd, o)
	return cmd
}

func (a *app) run(ctx context.Context, o *runOptions) error {
	if o.output != "text" && o.output != "json" {
		return fmt.Errorf("unknown output format %q (expected text or json)", o.output)
	}
	target, list, err := a.loadConfigs()
	if err != nil {
		return err
	}
	opts := runner.Options{Rules: o.rules, Tags: o.tags}
	if target.Dir {
		if err := checkRuleNames(list, o.rules, target.Path); err != nil {
			return err
		}
	}
	var configs []output.Config
	for _, l := range list {
		oc, ran, err := a.runConfig(ctx, l, opts, target.Dir)
		if ctx.Err() != nil {
			if err == nil {
				err = errors.New("run interrupted")
			}
			return err
		}
		if err != nil {
			if !target.Dir {
				return err
			}
			oc.Error = configError(l.path, err)
			fmt.Fprintf(a.stderr, "onecheck: %s\n", oc.Error)
		} else if !ran {
			continue
		}
		configs = append(configs, oc)
	}
	if err := output.Write(a.stdout, o.output, o.onlyFailed, target.Dir, configs); err != nil {
		return err
	}
	a.code = output.ExitCode(configs)
	return nil
}

// runConfig runs one config as a separate "onecheck -c <file>" would. ran is
// false when the --rule/--tag filter leaves nothing to run in this config.
func (a *app) runConfig(ctx context.Context, l loadedConfig, opts runner.Options, dir bool) (oc output.Config, ran bool, err error) {
	oc.Path = l.path
	if l.err != nil {
		return oc, false, l.err
	}
	cfg := l.cfg
	oc.Name = cfg.Name
	if dir {
		var ok bool
		if opts, ok = narrowOptions(cfg, opts); !ok {
			return oc, false, nil
		}
	}
	log, closer, err := a.logger(cfg)
	if err != nil {
		return oc, false, err
	}
	defer closer.Close()

	log.Debug("run started", "config", cfg.Path, "name", cfg.Name, "version", version.Version, "log_file", cfg.LogFile())
	results, err := runner.Run(ctx, cfg, opts, log)
	if err != nil {
		log.Error("run failed", "error", err)
		return oc, false, err
	}
	summary := output.Summarize(results)
	log.Info("run finished", "total", summary.Total, "passed", summary.Passed, "failed", summary.Failed, "errors", summary.Errors)
	oc.Results = results
	return oc, true, nil
}
