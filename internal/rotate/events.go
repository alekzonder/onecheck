package rotate

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Events removes expired events from the queue directory and returns how
// many files it removed. An event is expired when its expires_at has passed.
// Files that cannot be read as events, and leftover temp files, are removed
// once they are older than ttl. A missing directory is not an error.
func Events(dir string, ttl time.Duration, now time.Time) (int, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		name := e.Name()
		isEvent, isTemp := strings.HasSuffix(name, ".json"), strings.HasSuffix(name, ".tmp")
		if e.IsDir() || (!isEvent && !isTemp) {
			continue
		}
		path := filepath.Join(dir, name)
		if !expired(path, isEvent, ttl, now) {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func expired(path string, isEvent bool, ttl time.Duration, now time.Time) bool {
	if isEvent {
		var ev struct {
			ExpiresAt time.Time `json:"expires_at"`
		}
		data, err := os.ReadFile(path)
		if err == nil && json.Unmarshal(data, &ev) == nil && !ev.ExpiresAt.IsZero() {
			return now.After(ev.ExpiresAt)
		}
	}
	info, err := os.Stat(path)
	return err == nil && now.Sub(info.ModTime()) > ttl
}
