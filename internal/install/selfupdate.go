package install

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/alekzonder/onecheck/internal/github"
	"github.com/alekzonder/onecheck/internal/version"
)

// CoreRepo is where onecheck itself is released.
const CoreRepo = "alekzonder/onecheck"

// SelfUpdate replaces the onecheck binary at exe with the given release of
// repo (the newest stable one when wantVersion is empty). It returns the new
// version, or "" when exe is already at that version. The current binary is
// only replaced, atomically, after the download passed its checksum and the
// new binary proved it can run.
func SelfUpdate(ctx context.Context, c github.Client, repo, wantVersion, exe, goos, goarch string) (string, error) {
	releases, err := c.Releases(ctx, repo)
	if err != nil {
		return "", err
	}
	r, err := pick(releases, repo, wantVersion)
	if err != nil {
		return "", err
	}
	newVersion := trimV(r.Tag)
	if newVersion == version.Version {
		return "", nil
	}
	name := fmt.Sprintf("onecheck_%s_%s", goos, goarch)
	asset, ok := findAsset(r, name)
	if !ok {
		return "", fmt.Errorf("release %s of %s has no build for %s/%s (no asset %s)", r.Tag, repo, goos, goarch, name)
	}
	tmp, _, err := fetch(ctx, c, r, asset, tempDirFor(exe))
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp) // a no-op after the rename
	if out, err := exec.CommandContext(ctx, tmp, "version").CombinedOutput(); err != nil {
		return "", fmt.Errorf("downloaded onecheck %s does not run: %w: %s", r.Tag, err, out)
	}
	if err := os.Rename(tmp, exe); err != nil {
		return "", fmt.Errorf("cannot replace %s: %w", exe, err)
	}
	return newVersion, nil
}
