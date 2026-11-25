// Package cli implements the onecheck command line.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// Exit codes of onecheck.
const (
	exitOK     = 0
	exitFailed = 1 // at least one rule failed or could not be checked
	exitError  = 2 // onecheck itself could not do its job
)

const rootLong = `onecheck runs the checks described in a YAML config and reports whether
everything is fine. It is meant to be started by cron or a systemd timer.

Each rule names a module (file, http, command, ...) that collects data, and a
set of conditions onecheck evaluates on that data. For every rule onecheck
remembers its state between runs and writes an event when a rule goes from
pass to fail or recovers.

Running "onecheck" without a command is the same as "onecheck run".

Config, first match wins:
  1. --config <file or directory>
  2. $ONECHECK_CONFIG (a file or a directory)
  3. /etc/onecheck/            if it holds a config file
  4. $HOME/.config/onecheck/   if it holds a config file

With a directory, every *.yaml and *.yml file directly in it (subdirectories
such as conf.d/ are not read) is a config, and they run one after another as
separate runs would; the output combines them. So plain "onecheck" shows the
state of everything configured on the host.

Exit codes:
  0  every rule passed
  1  at least one rule failed or could not be checked
  2  onecheck could not run (bad config, a run already in progress, ...),
     or one config of a directory could not run

Documentation: https://github.com/alekzonder/onecheck/tree/main/docs`

const rootExample = `  # run every config of /etc/onecheck (or ~/.config/onecheck)
  onecheck

  # run every config of another directory
  onecheck -c ./configs

  # use a specific config and print only what is wrong, as JSON
  onecheck -c /etc/onecheck/web.yaml --only-failed -o json

  # check a config without running anything
  onecheck validate -c ./config.yaml

  # show where the config, log, state and events of this config are
  onecheck paths

  # install the bundled modules
  onecheck module install alekzonder/onecheck`

// app carries the state shared by all commands of one invocation.
type app struct {
	stdout io.Writer
	stderr io.Writer

	configPath string
	logLevel   string

	// code is the exit code set by a command that ran successfully.
	code int
}

// Execute runs onecheck with the given arguments and returns the exit code.
func Execute(args []string, stdout, stderr io.Writer) int {
	a := &app{stdout: stdout, stderr: stderr}
	root := a.rootCmd()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(stderr, "onecheck: %v\n", err)
		return exitError
	}
	return a.code
}

func (a *app) rootCmd() *cobra.Command {
	run := &runOptions{}
	root := &cobra.Command{
		Use:           "onecheck",
		Short:         "Run configured system checks and report what is broken",
		Long:          rootLong,
		Example:       rootExample,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd.Context(), run)
		},
	}
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return fmt.Errorf("%w\nSee '%s --help'", err, cmd.CommandPath())
	})
	root.PersistentFlags().StringVarP(&a.configPath, "config", "c", "", "config file or directory of configs (default: see above)")
	root.PersistentFlags().StringVar(&a.logLevel, "log-level", "", "log level: debug, info, warn or error; overrides the config and also prints the log on stderr")
	addRunFlags(root, run)

	root.AddCommand(a.runCmd(), a.validateCmd(), a.pathsCmd(), a.moduleCmd(), a.selfUpdateCmd(), a.logrotateCmd(), a.versionCmd())
	return root
}
