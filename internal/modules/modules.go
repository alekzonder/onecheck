// Package modules finds check modules on disk and runs them.
package modules

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alekzonder/onecheck/pkg/module"
)

// The range of module protocol versions this onecheck can talk to.
const (
	MinProtocol = 1
	MaxProtocol = 1
)

// Prefix is the file name prefix of every module executable.
const Prefix = "onecheck-"

var describeTimeout = 5 * time.Second

// ErrIncompatible means a module speaks a protocol version outside the
// supported range.
var ErrIncompatible = errors.New("incompatible protocol")

// Info is a module found on disk together with its self-description.
type Info struct {
	module.Description
	Path string
}

// Compatible reports whether onecheck supports the module's protocol version.
func (i *Info) Compatible() bool {
	return i.Protocol >= MinProtocol && i.Protocol <= MaxProtocol
}

// CompatError explains why an incompatible module cannot be used.
func (i *Info) CompatError() error {
	if i.Compatible() {
		return nil
	}
	return fmt.Errorf("%w: module %q speaks protocol %d, this onecheck supports %d..%d; update the module or onecheck",
		ErrIncompatible, i.Name, i.Protocol, MinProtocol, MaxProtocol)
}

// Path returns where the module with the given name lives in dir.
func Path(dir, name string) string {
	return filepath.Join(dir, Prefix+name)
}

// List describes every module executable in dir. A missing directory is an
// empty list. Files that are not usable modules are reported in errs.
func List(ctx context.Context, dir string) (infos []*Info, errs []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			errs = append(errs, err)
		}
		return nil, errs
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, Prefix) || strings.HasSuffix(name, ".tmp") {
			continue
		}
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
			continue
		}
		info, err := Describe(ctx, filepath.Join(dir, name))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		infos = append(infos, info)
	}
	sort.Slice(infos, func(a, b int) bool { return infos[a].Name < infos[b].Name })
	return infos, errs
}
