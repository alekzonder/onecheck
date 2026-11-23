package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/alekzonder/onecheck/internal/store"
)

// ManifestFile records where each installed module came from.
const ManifestFile = "installed.json"

// Installed is the origin of one installed module.
type Installed struct {
	Repo    string `json:"repo"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// Installed returns the manifest: module name → origin. Modules copied into
// the directory by hand are not in it.
func (m *Manager) Installed() (map[string]Installed, error) {
	out := map[string]Installed{}
	data, err := os.ReadFile(filepath.Join(m.Dir, ManifestFile))
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("%s is corrupt: %w", filepath.Join(m.Dir, ManifestFile), err)
	}
	return out, nil
}

func (m *Manager) saveInstalled(inst map[string]Installed) error {
	data, err := json.MarshalIndent(inst, "", "  ")
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(filepath.Join(m.Dir, ManifestFile), append(data, '\n'), 0o644)
}
