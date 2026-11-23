package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekzonder/onecheck/internal/github"
)

// ChecksumsFile is the release asset listing the sha256 of every other asset
// in "sha256sum" format.
const ChecksumsFile = "checksums.txt"

// trimV turns a tag such as "v1.2.0" into the version "1.2.0".
func trimV(tag string) string { return strings.TrimPrefix(tag, "v") }

// pick returns the release for version, or the newest stable one when
// version is empty.
func pick(releases []github.Release, repo, version string) (*github.Release, error) {
	for i := range releases {
		r := &releases[i]
		if version == "" && !r.Prerelease {
			return r, nil
		}
		if version != "" && trimV(r.Tag) == trimV(version) {
			return r, nil
		}
	}
	if version == "" {
		return nil, fmt.Errorf("%s has no stable release", repo)
	}
	return nil, fmt.Errorf("%s has no release %s", repo, version)
}

func findAsset(r *github.Release, name string) (github.Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return github.Asset{}, false
}

// checksum returns the expected sha256 of an asset from the release's
// checksums.txt.
func checksum(ctx context.Context, c github.Client, r *github.Release, asset string) (string, error) {
	sums, ok := findAsset(r, ChecksumsFile)
	if !ok {
		return "", fmt.Errorf("release %s has no %s; refusing to install unverified files", r.Tag, ChecksumsFile)
	}
	var buf bytes.Buffer
	if err := c.Download(ctx, sums, &buf); err != nil {
		return "", err
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("%s of release %s has no entry for %s", ChecksumsFile, r.Tag, asset)
}

// fetch downloads an asset into a new executable temp file in dir after
// verifying its checksum. The caller renames or removes the file.
func fetch(ctx context.Context, c github.Client, r *github.Release, asset github.Asset, dir string) (tmp, sum string, err error) {
	want, err := checksum(ctx, c, r, asset.Name)
	if err != nil {
		return "", "", err
	}
	f, err := os.CreateTemp(dir, "."+asset.Name+".*.tmp")
	if err != nil {
		return "", "", err
	}
	path := f.Name()
	defer func() {
		if err != nil {
			os.Remove(path)
		}
	}()
	hash := sha256.New()
	err = c.Download(ctx, asset, io.MultiWriter(f, hash))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", "", err
	}
	sum = hex.EncodeToString(hash.Sum(nil))
	if sum != want {
		return "", "", fmt.Errorf("checksum mismatch for %s: %s lists %s, downloaded file has %s",
			asset.Name, ChecksumsFile, want, sum)
	}
	if err = os.Chmod(path, 0o755); err != nil {
		return "", "", err
	}
	return path, sum, nil
}

// tempDirFor returns the directory a file will be replaced in; temp files
// must live there for the rename to be atomic.
func tempDirFor(path string) string { return filepath.Dir(path) }
