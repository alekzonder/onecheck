package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/alekzonder/onecheck/internal/install"
	"github.com/alekzonder/onecheck/internal/version"
)

const selfUpdateLong = `Replace the onecheck binary with a release from GitHub.

The release asset onecheck_<os>_<arch> is downloaded next to the running
binary, verified against the release's checksums.txt, started once to prove
it works, and then moved over the current binary in one step. If anything
fails, the current binary stays as it is.

A onecheck installed by install.sh is not updated this way: run install.sh
again, it installs the new version next to the old one and switches to it.

Modules are not touched; update them with "onecheck module update".
Set GITHUB_TOKEN to raise GitHub's rate limit.`

func (a *app) selfUpdateCmd() *cobra.Command {
	var want string
	cmd := &cobra.Command{
		Use:     "self-update",
		Short:   "Update onecheck itself to the latest release",
		Long:    selfUpdateLong,
		Example: "  onecheck self-update\n  onecheck self-update --version 1.4.0",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exe, err := selfUpdateTarget()
			if err != nil {
				return err
			}
			repo := install.CoreRepo
			if r := os.Getenv("ONECHECK_SELF_UPDATE_REPO"); r != "" {
				repo = r
			}
			newVersion, err := install.SelfUpdate(cmd.Context(), githubClient(), repo, want, exe, runtime.GOOS, runtime.GOARCH)
			if err != nil {
				return err
			}
			if newVersion == "" {
				fmt.Fprintf(a.stdout, "onecheck %s is already the requested version\n", version.Version)
				return nil
			}
			fmt.Fprintf(a.stdout, "updated onecheck %s -> %s (%s)\n", version.Version, newVersion, exe)
			return nil
		},
	}
	cmd.Flags().StringVar(&want, "version", "", "install this version instead of the latest stable one")
	return cmd
}

// selfUpdateTarget is the binary to replace: the running executable, with
// symlinks resolved. ONECHECK_SELF_UPDATE_TARGET overrides it.
func selfUpdateTarget() (string, error) {
	if target := os.Getenv("ONECHECK_SELF_UPDATE_TARGET"); target != "" {
		return target, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot find the running binary: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	return exe, checkSelfUpdatable(exe)
}

// checkSelfUpdatable refuses a binary that install.sh put into
// <root>/versions/<version>/onecheck: replacing it in place would leave a
// newer onecheck in the directory of an older version.
func checkSelfUpdatable(exe string) error {
	dir := filepath.Dir(exe)
	if filepath.Base(filepath.Dir(dir)) != "versions" {
		return nil
	}
	return fmt.Errorf("%s was installed by install.sh, which keeps each version in its own directory; "+
		"run install.sh again to install another version and switch to it (see docs/getting-started.md)", exe)
}
