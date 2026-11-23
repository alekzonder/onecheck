package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/alekzonder/onecheck/internal/config"
	"github.com/alekzonder/onecheck/internal/result"
	"github.com/alekzonder/onecheck/internal/state"
)

// ErrLocked means another run of the same config is still in progress.
var ErrLocked = errors.New("another onecheck run of this config is still in progress")

// ErrCorrupt means a state file exists but cannot be used.
var ErrCorrupt = errors.New("state file is corrupt")

const (
	lockFile  = ".lock"
	ownerFile = ".owner"
)

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// Store gives access to the files of one config.
type Store struct {
	cfg     *config.Config
	results string
	state   string
	events  string
}

// Open creates the directories of the config and returns its store.
func Open(cfg *config.Config) (*Store, error) {
	s := &Store{cfg: cfg, results: cfg.ResultsDir(), state: cfg.StateDir(), events: cfg.EventsDir()}
	for _, dir := range []string{s.results, s.state, s.events} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("cannot create directory: %w", err)
		}
	}
	return s, nil
}

// Lock takes the per-config run lock and checks that the state directory
// belongs to this config file. It returns ErrLocked when a run is in progress.
func (s *Store) Lock() (unlock func(), err error) {
	f, err := os.OpenFile(filepath.Join(s.state, lockFile), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, err
	}
	unlock = func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}
	if err := s.claim(); err != nil {
		unlock()
		return nil, err
	}
	return unlock, nil
}

// claim records which config file owns the state directory and refuses a
// different file using the same config name.
func (s *Store) claim() error {
	path := filepath.Join(s.state, ownerFile)
	data, err := os.ReadFile(path)
	if err == nil {
		if owner := strings.TrimSpace(string(data)); owner != "" && owner != s.cfg.Path {
			return fmt.Errorf("config name %q is already used by %s; %s must use a different name",
				s.cfg.Name, owner, s.cfg.Path)
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return WriteFileAtomic(path, []byte(s.cfg.Path+"\n"), 0o644)
}

// fileName maps a rule name to a file name that cannot leave the directory.
func fileName(rule string) string {
	name := unsafeChars.ReplaceAllString(rule, "_")
	if name == "" || name[0] == '.' {
		name = "_" + name
	}
	return name + ".json"
}

// LoadState returns the saved state of a rule: nil when there is none, and
// nil with ErrCorrupt when the file cannot be used.
func (s *Store) LoadState(rule string) (*state.State, error) {
	data, err := os.ReadFile(filepath.Join(s.state, fileName(rule)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st state.State
	if err := json.Unmarshal(data, &st); err != nil || (st.State != state.Pass && st.State != state.Fail) {
		return nil, fmt.Errorf("%w: %s", ErrCorrupt, filepath.Join(s.state, fileName(rule)))
	}
	return &st, nil
}

// SaveState stores the state of a rule.
func (s *Store) SaveState(rule string, st state.State) error {
	return writeJSON(filepath.Join(s.state, fileName(rule)), st)
}

// WriteResult stores the latest result of a rule, replacing the previous one.
func (s *Store) WriteResult(r result.Result) error {
	return writeJSON(filepath.Join(s.results, fileName(r.Rule)), r)
}

// WriteEvent puts a transition event into the queue.
func (s *Store) WriteEvent(e result.Event) error {
	return writeJSON(filepath.Join(s.events, fileName(e.ID)), e)
}

// Prune removes the result and state files of rules that are not in keep.
func (s *Store) Prune(keep []string) error {
	wanted := make(map[string]bool, len(keep))
	for _, rule := range keep {
		wanted[fileName(rule)] = true
	}
	for _, dir := range []string{s.results, s.state} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") || wanted[name] {
				continue
			}
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// EventID builds the id (and file name) of an event.
func EventID(at time.Time, cfg, rule, transition string) string {
	return fmt.Sprintf("%s-%s-%s-%s", at.UTC().Format("20060102T150405Z"), cfg, rule, transition)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(data, '\n'), 0o644)
}
