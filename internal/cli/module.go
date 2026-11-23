package cli

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/alekzonder/onecheck/internal/github"
	"github.com/alekzonder/onecheck/internal/install"
	"github.com/alekzonder/onecheck/internal/modules"
)

const moduleLong = `Manage check modules.

A module is an executable named onecheck-<name> in the modules directory
(paths.modules in the config). onecheck looks into that directory on every
run, so a module is usable as soon as it is installed: nothing to restart.

Modules are distributed as GitHub release assets named

  onecheck-<name>_<os>_<arch>      e.g. onecheck-http_linux_amd64

next to a checksums.txt with their sha256 sums. Every download is verified
against it, and a module is only activated after it reported a protocol
version this onecheck supports.

The modules directory comes from the config; use --modules-dir to manage
modules without a config. Set GITHUB_TOKEN to raise GitHub's rate limit or to
reach private repositories.`

type moduleOptions struct {
	dir string
}

func (a *app) moduleCmd() *cobra.Command {
	o := &moduleOptions{}
	cmd := &cobra.Command{
		Use:   "module",
		Short: "List, search, install, update and remove check modules",
		Long:  moduleLong,
		Example: `  onecheck module list
  onecheck module search alekzonder/onecheck
  onecheck module install alekzonder/onecheck
  onecheck module install alekzonder/onecheck http file@1.2.0
  onecheck module update
  onecheck module remove http`,
	}
	cmd.PersistentFlags().StringVar(&o.dir, "modules-dir", "", "modules directory (default: paths.modules of the config)")
	cmd.AddCommand(a.moduleListCmd(o), a.moduleSearchCmd(o), a.moduleInstallCmd(o), a.moduleUpdateCmd(o), a.moduleRemoveCmd(o))
	return cmd
}

// githubClient builds the release client. ONECHECK_GITHUB_API points it at
// another API endpoint (GitHub Enterprise, tests).
func githubClient() github.Client {
	return github.New(os.Getenv("ONECHECK_GITHUB_API"), os.Getenv("GITHUB_TOKEN"))
}

// manager resolves the modules directory and the default repositories.
func (a *app) manager(o *moduleOptions) (*install.Manager, []string, error) {
	m := &install.Manager{Client: githubClient(), Dir: o.dir, OS: runtime.GOOS, Arch: runtime.GOARCH}
	repos := []string{install.CoreRepo}
	cfg, err := a.loadConfig()
	if err == nil {
		repos = cfg.Modules.Repos
		if m.Dir == "" {
			m.Dir = cfg.Paths.Modules
		}
	} else if m.Dir == "" || a.configPath != "" {
		return nil, nil, fmt.Errorf("%w\n(or pass --modules-dir <dir> to manage modules without a config)", err)
	}
	return m, repos, nil
}

func (a *app) moduleListCmd(o *moduleOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "Show installed modules, their versions and compatibility",
		Example: "  onecheck module list",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, _, err := a.manager(o)
			if err != nil {
				return err
			}
			infos, errs := modules.List(cmd.Context(), m.Dir)
			for _, e := range errs {
				fmt.Fprintf(a.stderr, "onecheck: warning: %v\n", e)
			}
			if len(infos) == 0 {
				fmt.Fprintf(a.stdout, "no modules installed in %s\n", m.Dir)
				return nil
			}
			origin, err := m.Installed()
			if err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "modules in %s:\n", m.Dir)
			w := tabwriter.NewWriter(a.stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tVERSION\tPROTOCOL\tCOMPATIBLE\tSOURCE\tDESCRIPTION")
			for _, info := range infos {
				compatible, source := "yes", "-"
				if !info.Compatible() {
					compatible = fmt.Sprintf("no (supported: %d..%d)", modules.MinProtocol, modules.MaxProtocol)
				}
				if from, ok := origin[info.Name]; ok {
					source = from.Repo
				}
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\n", info.Name, info.Version, info.Protocol, compatible, source, info.Description.Description)
			}
			return w.Flush()
		},
	}
}

func (a *app) moduleSearchCmd(o *moduleOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "search [owner/repo...]",
		Short: "Show the releases of repositories and the modules found in them",
		Long: `Look through the GitHub releases of the given repositories and list every
module built for this platform. Without arguments the repositories from
modules.repos in the config are searched.`,
		Example: "  onecheck module search\n  onecheck module search alekzonder/onecheck someone/onecheck-postgres",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, repos, err := a.manager(o)
			if err != nil {
				return err
			}
			if len(args) > 0 {
				repos = args
			}
			found, err := m.Search(cmd.Context(), repos)
			if err != nil {
				return err
			}
			if len(found) == 0 {
				fmt.Fprintf(a.stdout, "no modules for %s/%s found in the releases of %v\n", m.OS, m.Arch, repos)
				return nil
			}
			w := tabwriter.NewWriter(a.stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "REPOSITORY\tRELEASE\tMODULE\tINSTALL WITH")
			for _, f := range found {
				fmt.Fprintf(w, "%s\t%s\t%s\tonecheck module install %s %s@%s\n", f.Repo, f.Tag, f.Name, f.Repo, f.Name, f.Tag)
			}
			return w.Flush()
		},
	}
}

func (a *app) moduleInstallCmd(o *moduleOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "install <owner/repo> [name[@version]...]",
		Short: "Install modules from the GitHub releases of a repository",
		Long: `Download modules from a repository's GitHub releases into the modules
directory. Without module names, every module of the latest stable release is
installed. "name" takes the latest stable release, "name@version" a specific
one. An already installed module is replaced.`,
		Example: `  onecheck module install alekzonder/onecheck
  onecheck module install alekzonder/onecheck http
  onecheck module install alekzonder/onecheck file@1.2.0 http@1.2.0`,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("which repository? usage: onecheck module install <owner/repo> [name[@version]...]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			m, _, err := a.manager(o)
			if err != nil {
				return err
			}
			found, err := m.Install(cmd.Context(), args[0], args[1:])
			for _, f := range found {
				if err == nil {
					fmt.Fprintf(a.stdout, "installed %s %s from %s into %s\n", f.Name, trimTag(f.Tag), f.Repo, m.Dir)
				}
			}
			return err
		},
	}
}

func (a *app) moduleUpdateCmd(o *moduleOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "update [name...]",
		Short: "Update modules to the latest release of their repository",
		Long: `Update the named modules, or all of them, to the latest stable release of the
repository they were installed from. A release whose module speaks a protocol
this onecheck does not support is skipped; update onecheck first
(onecheck self-update).`,
		Example: "  onecheck module update\n  onecheck module update http file",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, _, err := a.manager(o)
			if err != nil {
				return err
			}
			updated, skipped, err := m.Update(cmd.Context(), args)
			for _, f := range updated {
				fmt.Fprintf(a.stdout, "updated %s to %s\n", f.Name, trimTag(f.Tag))
			}
			for _, s := range skipped {
				fmt.Fprintf(a.stdout, "skipped %s\n", s)
			}
			if err == nil && len(updated) == 0 && len(skipped) == 0 {
				fmt.Fprintln(a.stdout, "no modules were installed with \"onecheck module install\"; nothing to update")
			}
			return err
		},
	}
}

func (a *app) moduleRemoveCmd(o *moduleOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>...",
		Short:   "Remove installed modules",
		Example: "  onecheck module remove http",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			m, _, err := a.manager(o)
			if err != nil {
				return err
			}
			if err := m.Remove(args); err != nil {
				return err
			}
			for _, name := range args {
				fmt.Fprintf(a.stdout, "removed %s\n", name)
			}
			return nil
		},
	}
}

func trimTag(tag string) string {
	if len(tag) > 1 && tag[0] == 'v' {
		return tag[1:]
	}
	return tag
}
