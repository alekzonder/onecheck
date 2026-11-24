package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// socket is one listening socket from a /proc/net table.
type socket struct {
	proto string
	addr  netip.Addr
	port  uint16
	inode uint64
}

// tcpListen is the state column of a listening TCP socket.
const tcpListen = "0A"

// parseSockets reads a /proc/net/{tcp,tcp6,udp,udp6} table and returns the
// sockets that accept traffic from anyone: TCP in the LISTEN state, UDP
// without a remote address.
func parseSockets(r io.Reader, proto string, v6 bool) ([]socket, error) {
	var out []socket
	sc := bufio.NewScanner(r)
	seenHeader := false
	for line := 1; sc.Scan(); line++ {
		if line == 1 {
			if !strings.Contains(sc.Text(), "local_address") {
				return nil, fmt.Errorf("line 1: not a socket table: %.60q", sc.Text())
			}
			seenHeader = true
			continue
		}
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		if len(f) < 10 {
			return nil, fmt.Errorf("line %d: %d columns, want at least 10", line, len(f))
		}
		local, port, err := parseEndpoint(f[1], v6)
		if err != nil {
			return nil, fmt.Errorf("line %d: local address: %w", line, err)
		}
		remote, _, err := parseEndpoint(f[2], v6)
		if err != nil {
			return nil, fmt.Errorf("line %d: remote address: %w", line, err)
		}
		inode, err := strconv.ParseUint(f[9], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: inode %q is not a number", line, f[9])
		}
		if proto == "tcp" && f[3] != tcpListen {
			continue
		}
		if proto == "udp" && !remote.IsUnspecified() {
			continue
		}
		out = append(out, socket{proto: proto, addr: local, port: port, inode: inode})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !seenHeader {
		return nil, fmt.Errorf("empty socket table")
	}
	return out, nil
}

// parseEndpoint decodes "0100007F:1F90". The kernel prints the address as
// 32-bit words in host byte order.
func parseEndpoint(s string, v6 bool) (netip.Addr, uint16, error) {
	hexAddr, hexPort, ok := strings.Cut(s, ":")
	want := 8
	if v6 {
		want = 32
	}
	if !ok || len(hexAddr) != want {
		return netip.Addr{}, 0, fmt.Errorf("%q is not a %d-digit address with a port", s, want)
	}
	raw := make([]byte, want/2)
	for i := 0; i < len(raw); i += 4 {
		word, err := strconv.ParseUint(hexAddr[i*2:i*2+8], 16, 32)
		if err != nil {
			return netip.Addr{}, 0, fmt.Errorf("%q is not a hex address", hexAddr)
		}
		binary.NativeEndian.PutUint32(raw[i:], uint32(word))
	}
	port, err := strconv.ParseUint(hexPort, 16, 16)
	if err != nil {
		return netip.Addr{}, 0, fmt.Errorf("%q is not a hex port", hexPort)
	}
	addr, _ := netip.AddrFromSlice(raw)
	return addr.Unmap(), uint16(port), nil
}

// scanProcesses finds the name of a process holding each socket inode by
// walking <root>/<pid>/fd. The lowest pid wins. Processes that cannot be
// read (other users' without root) are skipped.
func scanProcesses(root string, inodes map[uint64]bool) map[uint64]string {
	names := map[uint64]string{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return names
	}
	var pids []int
	for _, e := range entries {
		if pid, err := strconv.Atoi(e.Name()); err == nil && e.IsDir() {
			pids = append(pids, pid)
		}
	}
	slices.Sort(pids)
	for _, pid := range pids {
		if len(names) == len(inodes) {
			break
		}
		dir := filepath.Join(root, strconv.Itoa(pid))
		fds, err := os.ReadDir(filepath.Join(dir, "fd"))
		if err != nil {
			continue
		}
		comm := ""
		for _, fd := range fds {
			inode, ok := socketInode(filepath.Join(dir, "fd", fd.Name()))
			if !ok || !inodes[inode] {
				continue
			}
			if _, seen := names[inode]; seen {
				continue
			}
			if comm == "" {
				b, err := os.ReadFile(filepath.Join(dir, "comm"))
				if err != nil {
					break
				}
				comm = strings.TrimSpace(string(b))
			}
			names[inode] = comm
		}
	}
	return names
}

// socketInode reads an fd link of the form "socket:[12345]".
func socketInode(path string) (uint64, bool) {
	target, err := os.Readlink(path)
	if err != nil || !strings.HasPrefix(target, "socket:[") || !strings.HasSuffix(target, "]") {
		return 0, false
	}
	inode, err := strconv.ParseUint(target[len("socket:["):len(target)-1], 10, 64)
	return inode, err == nil
}
