package glob

import "testing"

func TestCompile(t *testing.T) {
	cases := []struct {
		pattern, s string
		want       bool
	}{
		{"app-*", "app-web", true},
		{"app-*", "app-", true},
		{"app-*", "xapp-web", false},
		{"registry.local/*", "registry.local/team/app:1", true},
		{"registry.local/*", "registryxlocal/a", false},
		{"a+b", "a+b", true},
		{"a+b", "aab", false},
		{"nginx", "nginx", true},
		{"nginx", "nginx-2", false},
		{"web-?", "web-1", true},
		{"web-?", "web-12", false},
		{"[a-c]x", "bx", true},
		{"[a-c]x", "dx", false},
		{"[!a]x", "bx", true},
		{"[!a]x", "ax", false},
		{"[]a]", "]", true},
		{"(x)|y", "(x)|y", true},
		{"(x)|y", "y", false},
		{"*", "line1\nline2", true},
	}
	for _, c := range cases {
		re, err := Compile(c.pattern)
		if err != nil {
			t.Fatalf("Compile(%q): %v", c.pattern, err)
		}
		if got := re.MatchString(c.s); got != c.want {
			t.Errorf("%q matches %q = %v, want %v", c.pattern, c.s, got, c.want)
		}
	}
}

func TestCompileMalformed(t *testing.T) {
	for _, p := range []string{"[", "app-[a", "[!"} {
		if _, err := Compile(p); err == nil {
			t.Errorf("Compile(%q) must fail", p)
		}
	}
}

func TestHas(t *testing.T) {
	for p, want := range map[string]bool{"nginx": false, "app-*": true, "web-?": true, "[ab]": true} {
		if Has(p) != want {
			t.Errorf("Has(%q) = %v", p, !want)
		}
	}
}
