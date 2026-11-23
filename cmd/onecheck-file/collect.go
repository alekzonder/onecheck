package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/alekzonder/onecheck/pkg/module"
)

type params struct {
	Path string `json:"path"`
}

func collect(ctx context.Context, req module.Request) (map[string]any, error) {
	var p params
	if err := module.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	if p.Path == "" {
		return nil, errors.New("invalid params: path is required")
	}
	out := map[string]any{}
	link, err := os.Lstat(p.Path)
	if errors.Is(err, fs.ErrNotExist) {
		out["exists"] = false
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	info := link
	if link.Mode()&fs.ModeSymlink != 0 {
		// type and target describe the link; everything else its target.
		out["type"] = "symlink"
		if target, err := os.Readlink(p.Path); err == nil {
			out["target"] = target
		}
		info, err = os.Stat(p.Path)
		if errors.Is(err, fs.ErrNotExist) {
			out["exists"] = false
			return out, nil
		}
		if err != nil {
			return nil, err
		}
	} else {
		out["type"] = typeOf(info)
	}
	out["exists"] = true
	out["age"] = time.Since(info.ModTime()).Seconds()
	out["mode"] = fmt.Sprintf("%04o", info.Mode().Perm())
	addOwner(out, info)
	if err := addContents(ctx, out, p.Path, info, req); err != nil {
		return nil, err
	}
	return out, nil
}

func typeOf(info fs.FileInfo) string {
	switch {
	case info.Mode().IsRegular():
		return "file"
	case info.IsDir():
		return "dir"
	}
	return "other"
}

func addOwner(out map[string]any, info fs.FileInfo) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	uid, gid := strconv.FormatUint(uint64(st.Uid), 10), strconv.FormatUint(uint64(st.Gid), 10)
	out["owner"], out["group"] = uid, gid
	if u, err := user.LookupId(uid); err == nil {
		out["owner"] = u.Username
	}
	if g, err := user.LookupGroupId(gid); err == nil {
		out["group"] = g.Name
	}
}

// addContents adds the fields that need more than a stat call, and only when
// the rule uses them.
func addContents(ctx context.Context, out map[string]any, path string, info fs.FileInfo, req module.Request) error {
	switch {
	case info.IsDir():
		if req.WantsOr("count", true) {
			entries, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			out["count"] = len(entries)
		}
		if req.WantsOr("size", false) {
			size, err := dirSize(ctx, path)
			if err != nil {
				return err
			}
			out["size"] = size
		}
	case info.Mode().IsRegular():
		out["size"] = info.Size()
		if req.WantsOr("content", false) {
			content, err := readLimited(path, req.MaxFieldSize)
			if err != nil {
				return err
			}
			out["content"] = content
		}
	default:
		out["size"] = info.Size()
	}
	return nil
}

func dirSize(ctx context.Context, root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// readLimited reads at most limit+1 bytes, so that the SDK can tell a value
// that fits from one that had to be truncated.
func readLimited(path string, limit int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var r io.Reader = f
	if limit > 0 {
		r = io.LimitReader(f, limit+1)
	}
	data, err := io.ReadAll(r)
	return string(data), err
}
