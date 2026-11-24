package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alekzonder/onecheck/pkg/module"
)

// The system is read through these variables so tests can replace it.
var (
	procRoot       = "/proc"
	interfaceAddrs = systemInterfaceAddrs
	processNames   = scanProcesses
)

type params struct {
	Proto     string `json:"proto"`
	Interface string `json:"interface"`
}

// table is one /proc/net file.
type table struct {
	file     string
	proto    string
	v6       bool
	required bool // IPv4 tables always exist on Linux; IPv6 ones only with IPv6 on
}

var tables = []table{
	{"net/tcp", "tcp", false, true},
	{"net/tcp6", "tcp", true, false},
	{"net/udp", "udp", false, true},
	{"net/udp6", "udp", true, false},
}

func collect(ctx context.Context, req module.Request) (map[string]any, error) {
	var p params
	if err := module.DecodeParams(req.Params, &p); err != nil {
		return nil, err
	}
	if p.Proto != "" && p.Proto != "tcp" && p.Proto != "udp" {
		return nil, fmt.Errorf("invalid params: proto must be tcp or udp, got %q", p.Proto)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ifaces, err := interfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}
	if _, known := ifaces[p.Interface]; p.Interface != "" && !known {
		return nil, fmt.Errorf("no network interface %q (interfaces: %s)",
			p.Interface, strings.Join(slices.Sorted(maps.Keys(ifaces)), ", "))
	}
	sockets, err := readSockets(p.Proto)
	if err != nil {
		return nil, err
	}
	groups := merge(selectSockets(sockets, p.Interface, ifaces))
	out := map[string]any{"count": int64(len(groups))}
	if !req.WantsOr("listening", true) {
		return out, nil
	}
	out["listening"] = elements(groups, ifaces)
	return out, nil
}

func readSockets(proto string) ([]socket, error) {
	var all []socket
	for _, t := range tables {
		if proto != "" && proto != t.proto {
			continue
		}
		path := filepath.Join(procRoot, t.file)
		f, err := os.Open(path)
		if errors.Is(err, fs.ErrNotExist) && !t.required {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w (the ports module works on Linux only)", path, err)
		}
		list, err := parseSockets(f, t.proto, t.v6)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		all = append(all, list...)
	}
	return all, nil
}

// selectSockets keeps the sockets reachable on the interface, or on any
// interface but loopback when none is named, sorted by proto, port, address.
func selectSockets(all []socket, iface string, ifaces map[string][]netip.Addr) []socket {
	kept := all[:0:0]
	for _, s := range all {
		switch {
		case s.addr.IsUnspecified():
		case iface == "" && s.addr.IsLoopback():
			continue
		case iface != "" && !owns(ifaces[iface], s.addr):
			continue
		}
		kept = append(kept, s)
	}
	slices.SortStableFunc(kept, func(a, b socket) int {
		return cmp.Or(cmp.Compare(a.proto, b.proto), cmp.Compare(a.port, b.port), a.addr.Compare(b.addr))
	})
	return kept
}

// owns reports whether an interface with these addresses has addr. A
// loopback interface has the whole loopback network (127.0.0.0/8), not only
// the address assigned to it.
func owns(addrs []netip.Addr, addr netip.Addr) bool {
	if slices.Contains(addrs, addr) {
		return true
	}
	return addr.IsLoopback() && slices.ContainsFunc(addrs, netip.Addr.IsLoopback)
}

// merge groups sorted sockets that share proto, address and port
// (SO_REUSEPORT, worker processes): each group is one listening port.
func merge(sorted []socket) [][]socket {
	var groups [][]socket
	for _, s := range sorted {
		if n := len(groups); n > 0 {
			if p := groups[n-1][0]; p.proto == s.proto && p.port == s.port && p.addr == s.addr {
				groups[n-1] = append(groups[n-1], s)
				continue
			}
		}
		groups = append(groups, []socket{s})
	}
	return groups
}

// elements builds the list, one element per group.
func elements(groups [][]socket, ifaces map[string][]netip.Addr) []map[string]any {
	inodes := map[uint64]bool{}
	for _, g := range groups {
		for _, s := range g {
			if s.inode != 0 { // kernel sockets have no inode
				inodes[s.inode] = true
			}
		}
	}
	names := map[uint64]string{}
	if len(inodes) > 0 {
		names = processNames(procRoot, inodes)
	}
	out := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		s, process := g[0], ""
		for _, member := range g {
			if process = names[member.inode]; process != "" {
				break
			}
		}
		family := "ipv4"
		if s.addr.Is6() {
			family = "ipv6"
		}
		out = append(out, map[string]any{
			"proto":     s.proto,
			"port":      int64(s.port),
			"address":   s.addr.String(),
			"family":    family,
			"interface": interfaceOf(s.addr, ifaces),
			"process":   process,
		})
	}
	return out
}

func interfaceOf(addr netip.Addr, ifaces map[string][]netip.Addr) string {
	if addr.IsUnspecified() {
		return "any"
	}
	for _, name := range slices.Sorted(maps.Keys(ifaces)) {
		if owns(ifaces[name], addr) {
			return name
		}
	}
	return ""
}

func systemInterfaceAddrs() (map[string][]netip.Addr, error) {
	list, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make(map[string][]netip.Addr, len(list))
	for _, i := range list {
		addrs, err := i.Addrs()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", i.Name, err)
		}
		out[i.Name] = nil
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if addr, ok := netip.AddrFromSlice(ipn.IP); ok {
				out[i.Name] = append(out[i.Name], addr.Unmap())
			}
		}
	}
	return out, nil
}
