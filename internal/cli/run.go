package cli

import (
	"context"
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
it is named by --rule or carries one of the --tag tags.`

const runExample = `  onecheck run
  onecheck run -c ./config.yaml --only-failed
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
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	log, closer, err := a.logger(cfg)
	if err != nil {
		return err
	}
	defer closer.Close()

	log.Debug("run started", "config", cfg.Path, "name", cfg.Name, "version", version.Version, "log_file", cfg.LogFile())
	results, err := runner.Run(ctx, cfg, runner.Options{Rules: o.rules, Tags: o.tags}, log)
	if err != nil {
		log.Error("run failed", "error", err)
		return err
	}
	summary := output.Summarize(results)
	log.Info("run finished", "total", summary.Total, "passed", summary.Passed, "failed", summary.Failed, "errors", summary.Errors)
	if err := output.Write(a.stdout, o.output, o.onlyFailed, results); err != nil {
		return err
	}
	a.code = output.ExitCode(results)
	return nil
}
