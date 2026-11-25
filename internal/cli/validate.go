package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alekzonder/onecheck/internal/runner"
)

const validateLong = `Check the config without running any check.

validate reports every problem at once: YAML syntax, unknown keys, duplicate
rule names, modules that are not installed or not compatible, params a module
does not accept, conditions on fields a module does not have, unknown
operators and bad operator arguments.

With a directory (--config <dir>, or no --config at all) every config in it
is checked, and a config that cannot be loaded does not stop the others.

It exits with 0 when every config is usable as is and with 2 otherwise.`

func (a *app) validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "validate",
		Short:   "Check the config without running anything",
		Long:    validateLong,
		Example: "  onecheck validate\n  onecheck validate -c ./config.yaml\n  onecheck validate -c /etc/onecheck",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target, list, err := a.loadConfigs()
			if err != nil {
				return err
			}
			bad := 0
			for _, l := range list {
				if err := a.validateOne(cmd.Context(), l, target.Dir); err != nil {
					if !target.Dir {
						return err
					}
					fmt.Fprintf(a.stderr, "onecheck: %v\n", err)
					bad++
				}
			}
			if bad > 0 {
				return fmt.Errorf("%d of %d configs in %s are not usable", bad, len(list), target.Path)
			}
			return nil
		},
	}
}

// validateOne reports the problems of one config; in a directory each line
// names the config file.
func (a *app) validateOne(ctx context.Context, l loadedConfig, dir bool) error {
	if l.err != nil {
		return errors.New(configError(l.path, l.err))
	}
	errs := runner.Validate(ctx, l.cfg)
	if len(errs) == 0 {
		fmt.Fprintf(a.stdout, "config OK: %d rules (%s)\n", len(l.cfg.Rules), l.cfg.Path)
		return nil
	}
	for _, e := range errs {
		if dir {
			fmt.Fprintf(a.stderr, "%s: %v\n", l.path, e)
		} else {
			fmt.Fprintln(a.stderr, e)
		}
	}
	return fmt.Errorf("%d rule(s) of %s are not usable", len(errs), l.cfg.Path)
}
