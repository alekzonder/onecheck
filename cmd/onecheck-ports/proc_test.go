package main

import (
	"net/netip"
	"strings"
	"testing"
)

const hdr = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

const tcp4 = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 101 1 0000000000000000 100 0 0 10 0
   1: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 102 1 0000000000000000 100 0 0 10 0
   2: 90151182:01BB 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 103 1 0000000000000000 100 0 0 10 0
   3: 90151182:D83E 08080808:01BB 01 00000000:00000000 02:000005EC 00000000  1000        0 104 2 0000000000000000 22 4 28 10 -1
`

const tcp6 = `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:0016 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 201 1 0000000000000000 100 0 0 10 0
   1: 406B022A103700200000000001000000:1F91 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 202 1 0000000000000000 100 0 0 10 0
   2: 0000000000000000FFFF00000300A8C0:2382 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 203 1 0000000000000000 100 0 0 10 0
   3: 00000000000000000000000001000000:0019 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 204 1 0000000000000000 100 0 0 10 0
`

const udp4 = `   sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops
 1764: 00000000:0249 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 301 2 0000000000000000 0
 3836: 90151182:EA61 00000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 302 2 0000000000000000 0
 4000: 90151182:C000 08080808:0035 01 00000000:00000000 00:00000000 00000000  1000        0 303 2 0000000000000000 0
`

func TestParseTCPKeepsOnlyListening(t *testing.T) {
	got, err := parseSockets(strings.NewReader(tcp4), "tcp", false)
	if err != nil {
		t.Fatal(err)
	}
	want := []socket{
		{"tcp", netip.MustParseAddr("127.0.0.1"), 8080, 101},
		{"tcp", netip.MustParseAddr("0.0.0.0"), 22, 102},
		{"tcp", netip.MustParseAddr("130.17.21.144"), 443, 103},
	}
	assertSockets(t, got, want)
}

func TestParseTCP6DecodesAddresses(t *testing.T) {
	got, err := parseSockets(strings.NewReader(tcp6), "tcp", true)
	if err != nil {
		t.Fatal(err)
	}
	want := []socket{
		{"tcp", netip.MustParseAddr("::"), 22, 201},
		{"tcp", netip.MustParseAddr("2a02:6b40:2000:3710::1"), 8081, 202},
		{"tcp", netip.MustParseAddr("192.168.0.3"), 9090, 203}, // ::ffff:192.168.0.3
		{"tcp", netip.MustParseAddr("::1"), 25, 204},
	}
	assertSockets(t, got, want)
}

func TestParseUDPSkipsConnected(t *testing.T) {
	got, err := parseSockets(strings.NewReader(udp4), "udp", false)
	if err != nil {
		t.Fatal(err)
	}
	want := []socket{
		{"udp", netip.MustParseAddr("0.0.0.0"), 585, 301},
		{"udp", netip.MustParseAddr("130.17.21.144"), 60001, 302},
	}
	assertSockets(t, got, want)
}

func TestParseMalformedLineIsError(t *testing.T) {
	for name, body := range map[string]string{
		"short line":  hdr + "   0: 0100007F:1F90 00000000:0000 0A\n",
		"bad address": hdr + "   0: ZZ00007F:1F90 00000000:0000 0A 0:0 00:0 0 0 0 1\n",
		"bad port":    hdr + "   0: 0100007F:XX 00000000:0000 0A 0:0 00:0 0 0 0 1\n",
		"no colon":    hdr + "   0: 0100007F 00000000:0000 0A 0:0 00:0 0 0 0 1\n",
		"v6 in v4":    hdr + "   0: 00000000000000000000000000000000:0016 00000000:0000 0A 0:0 00:0 0 0 0 1\n",
	} {
		if _, err := parseSockets(strings.NewReader(body), "tcp", false); err == nil || !strings.Contains(err.Error(), "line 2") {
			t.Errorf("%s: want an error naming line 2, got %v", name, err)
		}
	}
}

func TestParseEmptyTable(t *testing.T) {
	got, err := parseSockets(strings.NewReader(hdr), "tcp", false)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestParseWithoutHeaderIsError(t *testing.T) {
	for name, body := range map[string]string{"empty file": "", "not a table": "Permission denied\n"} {
		if _, err := parseSockets(strings.NewReader(body), "tcp", false); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func assertSockets(t *testing.T, got, want []socket) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d sockets %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("socket %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}
