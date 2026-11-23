// Package rotate implements "onecheck logrotate": rotation of the log file
// and removal of expired events.
package rotate

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alekzonder/onecheck/internal/config"
)

// rotated is one already rotated log: <file>.<index>[.gz].
type rotated struct {
	path  string
	index int
	gz    bool
}

// Logs rotates file when it is larger than r.MaxSize: file becomes file.1
// (file.1.gz with r.Compress) and older files move up by one. It then
// removes rotated files beyond r.Keep or older than r.MaxAge. It reports
// whether the log was rotated. A missing log is not an error.
func Logs(file string, r config.Rotate, now time.Time) (bool, error) {
	info, err := os.Stat(file)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	did := false
	if r.MaxSize > 0 && info.Size() > int64(r.MaxSize) {
		if err := rotate(file, r.Compress); err != nil {
			return false, fmt.Errorf("cannot rotate %s: %w", file, err)
		}
		did = true
	}
	return did, cleanup(file, r, now)
}

func rotate(file string, compress bool) error {
	old, err := list(file)
	if err != nil {
		return err
	}
	// Highest index first, so that no file is overwritten.
	sort.Slice(old, func(a, b int) bool { return old[a].index > old[b].index })
	for _, o := range old {
		if err := os.Rename(o.path, name(file, o.index+1, o.gz)); err != nil {
			return err
		}
	}
	if !compress {
		return os.Rename(file, name(file, 1, false))
	}
	if err := gzipFile(file, name(file, 1, true)); err != nil {
		return err
	}
	return os.Remove(file)
}

func cleanup(file string, r config.Rotate, now time.Time) error {
	old, err := list(file)
	if err != nil {
		return err
	}
	for _, o := range old {
		tooMany := o.index > r.Keep
		tooOld := false
		if r.MaxAge > 0 {
			if info, err := os.Stat(o.path); err == nil {
				tooOld = now.Sub(info.ModTime()) > time.Duration(r.MaxAge)
			}
		}
		if tooMany || tooOld {
			if err := os.Remove(o.path); err != nil {
				return err
			}
		}
	}
	return nil
}

func name(file string, index int, gz bool) string {
	n := file + "." + strconv.Itoa(index)
	if gz {
		n += ".gz"
	}
	return n
}

// list finds the rotated files that belong to file.
func list(file string) ([]rotated, error) {
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil {
		return nil, err
	}
	prefix := filepath.Base(file) + "."
	var out []rotated
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), prefix)
		if !ok {
			continue
		}
		num, gz := strings.CutSuffix(rest, ".gz")
		index, err := strconv.Atoi(num)
		if err != nil || index < 1 {
			continue
		}
		out = append(out, rotated{path: filepath.Join(filepath.Dir(file), e.Name()), index: index, gz: gz})
	}
	return out, nil
}

func gzipFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(tmp)
		}
	}()
	zw := gzip.NewWriter(out)
	if _, err = io.Copy(zw, in); err == nil {
		err = zw.Close()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if info, serr := in.Stat(); serr == nil {
		_ = os.Chtimes(tmp, info.ModTime(), info.ModTime())
	}
	return os.Rename(tmp, dst)
}
