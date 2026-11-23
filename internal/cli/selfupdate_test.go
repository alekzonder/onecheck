package cli

import (
	"strings"
	"testing"
)

func TestSelfUpdateRefusesVersionedInstall(t *testing.T) {
	err := checkSelfUpdatable("/usr/local/lib/onecheck/versions/0.1.0/onecheck")
	if err == nil {
		t.Fatal("self-update of a binary installed by install.sh was allowed")
	}
	if !strings.Contains(err.Error(), "install.sh") {
		t.Errorf("error does not point to install.sh: %v", err)
	}
}

func TestSelfUpdateAllowsPlainInstall(t *testing.T) {
	for _, path := range []string{"/usr/local/bin/onecheck", "/opt/versions/onecheck", "/home/u/onecheck/dist/onecheck"} {
		if err := checkSelfUpdatable(path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}
