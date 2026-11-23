package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alekzonder/onecheck/internal/runner"
)

const validateLong = `Check the config without running any check.

validate reports every problem at once: YAML syntax, unknown keys, duplicate
rule names, modules that are not installed or not compatible, params a module
does not accept, conditions on fields a module does not have, unknown
operators and bad operator arguments.

It exits with 0 when the config is usable as is and with 2 otherwise.`

func (a *app) validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "validate",
		Short:   "Check the config without running anything",
		Long:    validateLong,
		Example: "  onecheck validate\n  onecheck validate -c ./config.yaml",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			errs := runner.Validate(cmd.Context(), cfg)
			if len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintln(a.stderr, e)
				}
				return fmt.Errorf("%d rule(s) of %s are not usable", len(errs), cfg.Path)
			}
			fmt.Fprintf(a.stdout, "config OK: %d rules (%s)\n", len(cfg.Rules), cfg.Path)
			return nil
		},
	}
}
