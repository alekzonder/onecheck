// Package store reads and writes onecheck's files: per-config state and
// results, and the shared event queue.
package store

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path so that readers see either the old or
// the new content, never a partial file: temp file in the same directory,
// then rename.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(perm)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}
