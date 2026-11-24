package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/onecheck/pkg/module"
)

// fakeProc builds a /proc tree in a temp dir and points the module at it.
func fakeProc(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldRoot, oldIf := procRoot, interfaceAddrs
	procRoot = root
	interfaceAddrs = func() (map[string][]netip.Addr, error) {
		return map[string][]netip.Addr{
			"lo":   {netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("::1")},
			"eth0": {netip.MustParseAddr("130.17.21.144"), netip.MustParseAddr("2a02:6b40:2000:3710::1")},
			"wg0":  {netip.MustParseAddr("192.168.0.3")},
		}, nil
	}
	t.Cleanup(func() { procRoot, interfaceAddrs = oldRoot, oldIf })
	return root
}

func allTables() map[string]string {
	return map[string]string{"net/tcp": tcp4, "net/tcp6": tcp6, "net/udp": udp4, "net/udp6": "header\n"}
}

// addProcess gives pid a comm and fds pointing at socket inodes.
func addProcess(t *testing.T, root string, pid int, comm string, inodes ...int) {
	t.Helper()
	dir := filepath.Join(root, fmt.Sprint(pid), "fd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, fmt.Sprint(pid), "comm"), []byte(comm+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i, ino := range inodes {
		if err := os.Symlink(fmt.Sprintf("socket:[%d]", ino), filepath.Join(dir, fmt.Sprint(i+3))); err != nil {
			t.Fatal(err)
		}
	}
}

func run(t *testing.T, params string, fields ...string) (map[string]any, error) {
	t.Helper()
	return collect(context.Background(), module.Request{Params: json.RawMessage(params), Fields: fields})
}

// keys renders the list as "proto address:port@interface" for comparison.
func keys(t *testing.T, out map[string]any) []string {
	t.Helper()
	list, ok := out["listening"].([]map[string]any)
	if !ok {
		t.Fatalf("listening is %T", out["listening"])
	}
	res := make([]string, 0, len(list))
	for _, e := range list {
		res = append(res, fmt.Sprintf("%s %s:%d@%s", e["proto"], e["address"], e["port"], e["interface"]))
	}
	return res
}

func assertKeys(t *testing.T, out map[string]any, want ...string) {
	t.Helper()
	got := keys(t, out)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if out["count"] != int64(len(want)) {
		t.Errorf("count = %v, want %d", out["count"], len(want))
	}
}

func TestCollectDropsLoopbackByDefault(t *testing.T) {
	fakeProc(t, allTables())
	out, err := run(t, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	assertKeys(t, out,
		"tcp 0.0.0.0:22@any",
		"tcp :::22@any",
		"tcp 130.17.21.144:443@eth0",
		"tcp 2a02:6b40:2000:3710::1:8081@eth0",
		"tcp 192.168.0.3:9090@wg0",
		"udp 0.0.0.0:585@any",
		"udp 130.17.21.144:60001@eth0",
	)
}

func TestCollectElementShape(t *testing.T) {
	root := fakeProc(t, allTables())
	addProcess(t, root, 700, "sshd", 102, 201)
	addProcess(t, root, 900, "nginx", 103)
	addProcess(t, root, 800, "nginx-master", 103) // lower pid wins
	out, err := run(t, `{"proto":"tcp","interface":"eth0"}`)
	if err != nil {
		t.Fatal(err)
	}
	list := out["listening"].([]map[string]any)
	want := []map[string]any{
		{"proto": "tcp", "port": int64(22), "address": "0.0.0.0", "family": "ipv4", "interface": "any", "process": "sshd"},
		{"proto": "tcp", "port": int64(22), "address": "::", "family": "ipv6", "interface": "any", "process": "sshd"},
		{"proto": "tcp", "port": int64(443), "address": "130.17.21.144", "family": "ipv4", "interface": "eth0", "process": "nginx-master"},
		{"proto": "tcp", "port": int64(8081), "address": "2a02:6b40:2000:3710::1", "family": "ipv6", "interface": "eth0", "process": ""},
	}
	if len(list) != len(want) {
		t.Fatalf("got %d elements: %v", len(list), list)
	}
	for i := range want {
		if fmt.Sprint(list[i]) != fmt.Sprint(want[i]) {
			t.Errorf("element %d:\n got %v\nwant %v", i, list[i], want[i])
		}
	}
}

func TestCollectInterfaceLoopback(t *testing.T) {
	fakeProc(t, allTables())
	out, err := run(t, `{"interface":"lo","proto":"tcp"}`)
	if err != nil {
		t.Fatal(err)
	}
	assertKeys(t, out,
		"tcp 0.0.0.0:22@any",
		"tcp :::22@any",
		"tcp ::1:25@lo",
		"tcp 127.0.0.1:8080@lo",
	)
}

func TestCollectProtoUDP(t *testing.T) {
	fakeProc(t, allTables())
	out, err := run(t, `{"proto":"udp"}`)
	if err != nil {
		t.Fatal(err)
	}
	assertKeys(t, out, "udp 0.0.0.0:585@any", "udp 130.17.21.144:60001@eth0")
}

func TestCollectMergesReusePortSockets(t *testing.T) {
	tables := allTables()
	tables["net/tcp"] = tcp4 + "   9: 90151182:01BB 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 999 1 0000000000000000 100 0 0 10 0\n"
	root := fakeProc(t, tables)
	addProcess(t, root, 50, "worker", 999)
	out, err := run(t, `{"proto":"tcp","interface":"eth0"}`)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range out["listening"].([]map[string]any) {
		if e["port"] == int64(443) {
			n++
			if e["process"] != "worker" {
				t.Errorf("process = %v, want the one that can be seen", e["process"])
			}
		}
	}
	if n != 1 {
		t.Errorf("port 443 listed %d times", n)
	}
}

func TestCollectAddressWithoutInterface(t *testing.T) {
	tables := allTables()
	tables["net/tcp"] = "header\n   0: 0A00000A:0050 00000000:0000 0A 0:0 00:0 0 0 0 5\n" // 10.0.0.10
	tables["net/tcp6"] = "header\n"
	tables["net/udp"] = "header\n"
	fakeProc(t, tables)
	out, err := run(t, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	assertKeys(t, out, "tcp 10.0.0.10:80@")
}

func TestCollectCountOnlySkipsProcesses(t *testing.T) {
	root := fakeProc(t, allTables())
	// A pid directory that would fail the test if it were read as a process.
	if err := os.MkdirAll(filepath.Join(root, "700"), 0o755); err != nil {
		t.Fatal(err)
	}
	scanned := false
	old := processNames
	processNames = func(string, map[uint64]bool) map[uint64]string { scanned = true; return nil }
	t.Cleanup(func() { processNames = old })
	out, err := run(t, `{}`, "count")
	if err != nil {
		t.Fatal(err)
	}
	if scanned {
		t.Error("process names were looked up for a count-only request")
	}
	if _, has := out["listening"]; has || out["count"] != int64(7) {
		t.Errorf("out = %v", out)
	}
}

func TestCollectMissingTCPIsError(t *testing.T) {
	fakeProc(t, map[string]string{"net/udp": udp4})
	_, err := run(t, `{}`)
	if err == nil || !strings.Contains(err.Error(), "net/tcp") || !strings.Contains(err.Error(), "Linux") {
		t.Fatalf("err = %v", err)
	}
}

func TestCollectMissingUDPIsError(t *testing.T) {
	fakeProc(t, map[string]string{"net/tcp": tcp4})
	if _, err := run(t, `{}`); err == nil || !strings.Contains(err.Error(), "net/udp") {
		t.Fatalf("err = %v", err)
	}
}

func TestCollectWithoutIPv6(t *testing.T) {
	fakeProc(t, map[string]string{"net/tcp": tcp4, "net/udp": udp4})
	out, err := run(t, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	assertKeys(t, out,
		"tcp 0.0.0.0:22@any",
		"tcp 130.17.21.144:443@eth0",
		"udp 0.0.0.0:585@any",
		"udp 130.17.21.144:60001@eth0",
	)
}

func TestCollectMalformedTableIsError(t *testing.T) {
	tables := allTables()
	tables["net/tcp6"] = "header\n garbage\n"
	fakeProc(t, tables)
	_, err := run(t, `{}`)
	if err == nil || !strings.Contains(err.Error(), "net/tcp6") || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v", err)
	}
}

func TestCollectUnknownInterfaceIsError(t *testing.T) {
	fakeProc(t, allTables())
	_, err := run(t, `{"interface":"eth9"}`)
	if err == nil || !strings.Contains(err.Error(), `"eth9"`) || !strings.Contains(err.Error(), "eth0, lo, wg0") {
		t.Fatalf("err = %v", err)
	}
}

func TestCollectBadParams(t *testing.T) {
	fakeProc(t, allTables())
	for _, p := range []string{`{"proto":"sctp"}`, `{"port":22}`, `{"interface":7}`} {
		if _, err := run(t, p); err == nil || !strings.Contains(err.Error(), "invalid params") {
			t.Errorf("params %s: err = %v", p, err)
		}
	}
}

func TestCollectCancelled(t *testing.T) {
	fakeProc(t, allTables())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := collect(ctx, module.Request{Params: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("want an error for a cancelled context")
	}
}

func TestProcessNamesIgnoresUnreadable(t *testing.T) {
	root := t.TempDir()
	addProcess(t, root, 10, "a", 1)
	// not a pid, a pid without fd, a pid without comm
	for _, d := range []string{"net", "11", "12/fd"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("socket:[2]", filepath.Join(root, "12/fd/3")); err != nil {
		t.Fatal(err)
	}
	got := scanProcesses(root, map[uint64]bool{1: true, 2: true, 3: true})
	if len(got) != 1 || got[1] != "a" {
		t.Errorf("got %v", got)
	}
}

func TestDescribeListsFields(t *testing.T) {
	var out bytes.Buffer
	if code := module.Run(mod, []string{"describe"}, strings.NewReader(""), &out, &out); code != 0 {
		t.Fatalf("describe exited %d: %s", code, out.String())
	}
	var d module.Description
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Name != "ports" || d.Fields["listening"].Type != module.TypeArray || d.Fields["count"].Type != module.TypeNumber {
		t.Errorf("description = %+v", d)
	}
}
