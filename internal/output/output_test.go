package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alekzonder/onecheck/internal/match"
	"github.com/alekzonder/onecheck/internal/result"
)

func sample() []result.Result {
	tr := "fail"
	errMsg := `module "ghost" is not installed in /x`
	return []result.Result{
		{Rule: "api", Module: "http", Status: result.Pass, State: "pass", DurationMS: 124, Consecutive: 5},
		{Rule: "disk", Module: "file", Status: result.Fail, State: "fail", Transition: &tr, DurationMS: 3,
			Failures: []match.Failure{{Message: "size: expected lt \"10MB\", got 20971520"}}},
		{Rule: "soft", Module: "file", Status: result.Fail, State: "pass", Consecutive: 2, FailAfter: 3,
			Failures: []match.Failure{{Message: "exists: expected equals true, got false"}}},
		{Rule: "ghost", Module: "ghost", Status: result.Error, State: "fail", Error: &errMsg},
	}
}

func TestTextAll(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "text", false, sample()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"PASS", "api", "(http, 124ms)",
		"FAIL", "disk", "[transition: fail]", `size: expected lt "10MB", got 20971520`,
		"soft", "[state: pass, 2/3]",
		"ERROR", `module "ghost" is not installed in /x`,
		"4 rules: 1 passed, 2 failed, 1 errors",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text output lacks %q:\n%s", want, out)
		}
	}
}

func TestTextOnlyFailed(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "text", true, sample()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "api") {
		t.Errorf("passing rules must be hidden:\n%s", out)
	}
	if !strings.Contains(out, "disk") || !strings.Contains(out, "4 rules: 1 passed, 2 failed, 1 errors") {
		t.Errorf("failures and the full summary must stay:\n%s", out)
	}
}

func TestJSONShape(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "json", true, sample()); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Summary map[string]int   `json:"summary"`
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b.String())
	}
	if doc.Summary["total"] != 4 || doc.Summary["passed"] != 1 || doc.Summary["failed"] != 2 || doc.Summary["errors"] != 1 {
		t.Errorf("summary = %v", doc.Summary)
	}
	if len(doc.Results) != 3 || doc.Results[0]["rule"] != "disk" {
		t.Errorf("only-failed JSON must list the 3 non-passing rules: %v", doc.Results)
	}
	b.Reset()
	if err := Write(&b, "json", false, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"results": []`) {
		t.Errorf("empty results must be [], got %s", b.String())
	}
}

func TestUnknownFormat(t *testing.T) {
	if err := Write(&bytes.Buffer{}, "xml", false, nil); err == nil {
		t.Error("unknown format must be an error")
	}
}

func TestExitCode(t *testing.T) {
	all := sample()
	if ExitCode(all[:1]) != 0 || ExitCode(nil) != 0 {
		t.Error("all passing (or nothing to run) must be 0")
	}
	if ExitCode(all[:2]) != 1 || ExitCode(all[3:]) != 1 {
		t.Error("any fail or error must be 1")
	}
	if ExitCode(all[2:3]) != 1 {
		t.Error("the exit code follows the raw status of this run, not the confirmed state")
	}
}
