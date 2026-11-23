package units

import "testing"

func TestParseBytes(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		err  bool
	}{
		{"10MB", 10 * 1024 * 1024, false},
		{"1.5KB", 1536, false},
		{"7", 7, false},
		{"2 gb", 2 * 1024 * 1024 * 1024, false},
		{"1TB", 1 << 40, false},
		{"5B", 5, false},
		{"x", 0, true},
		{"", 0, true},
		{"10XB", 0, true},
		{"-1MB", 0, true},
	}
	for _, c := range cases {
		got, err := ParseBytes(c.in)
		if (err != nil) != c.err {
			t.Errorf("ParseBytes(%q) err = %v, want err %v", c.in, err, c.err)
			continue
		}
		if !c.err && got != c.want {
			t.Errorf("ParseBytes(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		err  bool
	}{
		{"1h", 3600, false},
		{"30d", 2592000, false},
		{"1.5", 1.5, false},
		{"2w", 1209600, false},
		{"1h30m", 5400, false},
		{"1d12h", 129600, false},
		{"500ms", 0.5, false},
		{"abc", 0, true},
		{"", 0, true},
	}
	for _, c := range cases {
		got, err := ParseDuration(c.in)
		if (err != nil) != c.err {
			t.Errorf("ParseDuration(%q) err = %v, want err %v", c.in, err, c.err)
			continue
		}
		if !c.err && got != c.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseBytesShortUnits(t *testing.T) {
	for in, want := range map[string]float64{"1k": 1024, "10K": 10240, "2m": 2 << 20, "1g": 1 << 30} {
		got, err := ParseBytes(in)
		if err != nil || got != want {
			t.Errorf("ParseBytes(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
}
