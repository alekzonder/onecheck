// Package install manages check modules and onecheck itself from GitHub
// Releases: search, install, update, remove, self-update.
package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/alekzonder/onecheck/internal/github"
	"github.com/alekzonder/onecheck/internal/modules"
)

var moduleNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Found is a module available in a release.
type Found struct {
	Repo  string
	Tag   string
	Name  string
	Asset github.Asset
}

// Manager installs modules into Dir for the platform OS/Arch.
type Manager struct {
	Client github.Client
	Dir    string
	OS     string
	Arch   string
}

func (m *Manager) suffix() string { return "_" + m.OS + "_" + m.Arch }

// assetName is how a module's release asset is named for this platform.
func (m *Manager) assetName(module string) string {
	return modules.Prefix + module + m.suffix()
}

// modulesIn lists the modules a release offers for this platform, by name.
func (m *Manager) modulesIn(repo string, r *github.Release) []Found {
	var out []Found
	for _, a := range r.Assets {
		name, ok := strings.CutPrefix(a.Name, modules.Prefix)
		if !ok {
			continue
		}
		name, ok = strings.CutSuffix(name, m.suffix())
		if !ok || !moduleNameRE.MatchString(name) {
			continue
		}
		out = append(out, Found{Repo: repo, Tag: r.Tag, Name: name, Asset: a})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// Search lists the modules found in the releases of the repositories,
// newest release first.
func (m *Manager) Search(ctx context.Context, repos []string) ([]Found, error) {
	var out []Found
	for _, repo := range repos {
		releases, err := m.Client.Releases(ctx, repo)
		if err != nil {
			return nil, err
		}
		for i := range releases {
			out = append(out, m.modulesIn(repo, &releases[i])...)
		}
	}
	return out, nil
}

// Install installs modules from a repository. Each spec is "name" (latest
// stable release) or "name@version". Without specs it installs every module
// of the latest stable release.
func (m *Manager) Install(ctx context.Context, repo string, specs []string) ([]Found, error) {
	releases, err := m.Client.Releases(ctx, repo)
	if err != nil {
		return nil, err
	}
	var todo []Found
	if len(specs) == 0 {
		r, err := pick(releases, repo, "")
		if err != nil {
			return nil, err
		}
		todo = m.modulesIn(repo, r)
		if len(todo) == 0 {
			return nil, fmt.Errorf("release %s of %s has no modules for %s/%s (expected assets named %s)",
				r.Tag, repo, m.OS, m.Arch, m.assetName("<name>"))
		}
	}
	for _, spec := range specs {
		name, version, _ := strings.Cut(spec, "@")
		if !moduleNameRE.MatchString(name) {
			return nil, fmt.Errorf("invalid module name %q", name)
		}
		r, err := pick(releases, repo, version)
		if err != nil {
			return nil, err
		}
		asset, ok := findAsset(r, m.assetName(name))
		if !ok {
			return nil, fmt.Errorf("release %s of %s has no module %q for %s/%s (no asset %s)",
				r.Tag, repo, name, m.OS, m.Arch, m.assetName(name))
		}
		todo = append(todo, Found{Repo: repo, Tag: r.Tag, Name: name, Asset: asset})
	}
	for _, f := range todo {
		r, _ := pick(releases, repo, f.Tag)
		if err := m.installOne(ctx, r, f); err != nil {
			return nil, fmt.Errorf("module %s %s: %w", f.Name, f.Tag, err)
		}
	}
	return todo, nil
}

// installOne downloads, verifies and activates one module. Until the final
// rename the previously installed module stays in place.
func (m *Manager) installOne(ctx context.Context, r *github.Release, f Found) error {
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return err
	}
	tmp, sum, err := fetch(ctx, m.Client, r, f.Asset, m.Dir)
	if err != nil {
		return err
	}
	defer os.Remove(tmp) // a no-op after the rename
	info, err := modules.Describe(ctx, tmp)
	if err != nil {
		return fmt.Errorf("downloaded file is not a working module: %w", err)
	}
	if info.Name != f.Name {
		return fmt.Errorf("asset %s contains module %q, expected %q", f.Asset.Name, info.Name, f.Name)
	}
	if err := info.CompatError(); err != nil {
		return err
	}
	if err := os.Rename(tmp, modules.Path(m.Dir, f.Name)); err != nil {
		return err
	}
	inst, err := m.Installed()
	if err != nil {
		return err
	}
	inst[f.Name] = Installed{Repo: f.Repo, Version: trimV(f.Tag), SHA256: sum}
	return m.saveInstalled(inst)
}

// Update brings the named modules (all recorded ones without names) to the
// latest stable release of the repository they were installed from. Modules
// that cannot or need not be updated are explained in skipped.
func (m *Manager) Update(ctx context.Context, names []string) (updated []Found, skipped []string, err error) {
	inst, err := m.Installed()
	if err != nil {
		return nil, nil, err
	}
	if len(names) == 0 {
		for name := range inst {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	cache := map[string][]github.Release{}
	for _, name := range names {
		origin, ok := inst[name]
		if !ok {
			skipped = append(skipped, fmt.Sprintf("%s: not installed with \"onecheck module install\", its source is unknown", name))
			continue
		}
		releases, ok := cache[origin.Repo]
		if !ok {
			if releases, err = m.Client.Releases(ctx, origin.Repo); err != nil {
				return updated, skipped, err
			}
			cache[origin.Repo] = releases
		}
		f, reason := m.updateOne(ctx, releases, name, origin)
		if reason != "" {
			skipped = append(skipped, name+": "+reason)
			continue
		}
		updated = append(updated, f)
	}
	return updated, skipped, nil
}

func (m *Manager) updateOne(ctx context.Context, releases []github.Release, name string, origin Installed) (Found, string) {
	r, err := pick(releases, origin.Repo, "")
	if err != nil {
		return Found{}, err.Error()
	}
	if trimV(r.Tag) == origin.Version {
		return Found{}, "up to date (" + origin.Version + ")"
	}
	asset, ok := findAsset(r, m.assetName(name))
	if !ok {
		return Found{}, fmt.Sprintf("release %s of %s no longer has this module", r.Tag, origin.Repo)
	}
	f := Found{Repo: origin.Repo, Tag: r.Tag, Name: name, Asset: asset}
	if err := m.installOne(ctx, r, f); err != nil {
		return Found{}, fmt.Sprintf("release %s not installed: %v", r.Tag, err)
	}
	return f, ""
}

// Remove deletes installed modules.
func (m *Manager) Remove(names []string) error {
	inst, err := m.Installed()
	if err != nil {
		return err
	}
	for _, name := range names {
		if !moduleNameRE.MatchString(name) {
			return fmt.Errorf("invalid module name %q", name)
		}
		err := os.Remove(modules.Path(m.Dir, name))
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("module %q is not installed in %s", name, m.Dir)
		}
		if err != nil {
			return err
		}
		delete(inst, name)
	}
	return m.saveInstalled(inst)
}
