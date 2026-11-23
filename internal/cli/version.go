package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alekzonder/onecheck/internal/modules"
	"github.com/alekzonder/onecheck/internal/version"
)

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the onecheck version and the module protocol it supports",
		Args:  cobra.NoArgs,
		Run: func(*cobra.Command, []string) {
			fmt.Fprintf(a.stdout, "onecheck %s (%s)\n", version.Version, protocolRange())
		},
	}
}

func protocolRange() string {
	if modules.MinProtocol == modules.MaxProtocol {
		return fmt.Sprintf("protocol %d", modules.MinProtocol)
	}
	return fmt.Sprintf("protocol %d..%d", modules.MinProtocol, modules.MaxProtocol)
}
