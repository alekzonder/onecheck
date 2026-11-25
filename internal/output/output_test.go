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

// one wraps results as the only config of a run.
func one(rs []result.Result) []Config {
	return []Config{{Name: "main", Path: "/p/config.yaml", Results: rs}}
}

func grouped() []Config {
	all := sample()
	return []Config{
		{Name: "main", Path: "/p/a.yaml", Results: all[:1]},
		{Name: "web", Path: "/p/b.yaml", Results: all[1:2]},
		{Path: "/p/bad.yaml", Error: `/p/bad.yaml: "name" is required`},
	}
}

func TestTextNotGroupedHasNoHeaders(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "text", false, false, one(sample())); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "==") || strings.Contains(b.String(), "configs") {
		t.Errorf("a single config file must print as before:\n%s", b.String())
	}
}

func TestWriteGrouped(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "text", false, true, grouped()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"== main  /p/a.yaml\nPASS",
		"== web  /p/b.yaml\nFAIL",
		"== /p/bad.yaml\nERROR  config: /p/bad.yaml: \"name\" is required\n",
		"2 rules: 1 passed, 1 failed, 0 errors; 1 of 3 configs could not run\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("grouped output lacks %q:\n%s", want, out)
		}
	}
}

func TestWriteGroupedOnlyFailed(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "text", true, true, grouped()); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "main") {
		t.Errorf("a config with nothing to show must be left out:\n%s", out)
	}
	if !strings.Contains(out, "== web") || !strings.Contains(out, "== /p/bad.yaml") {
		t.Errorf("failing and broken configs must stay:\n%s", out)
	}
	b.Reset()
	cfgs := grouped()[:1]
	if err := Write(&b, "text", false, true, cfgs); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "could not run") {
		t.Errorf("no config error, no note:\n%s", b.String())
	}
}

func TestWriteJSONConfigs(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "json", false, true, grouped()); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Summary map[string]int      `json:"summary"`
		Configs []map[string]string `json:"configs"`
		Results []map[string]any    `json:"results"`
	}
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b.String())
	}
	if len(doc.Configs) != 3 || doc.Configs[0]["name"] != "main" || doc.Configs[0]["path"] != "/p/a.yaml" {
		t.Errorf("configs = %v", doc.Configs)
	}
	if _, has := doc.Configs[0]["error"]; has {
		t.Errorf("a config that ran has no error key: %v", doc.Configs[0])
	}
	if doc.Configs[2]["error"] == "" || len(doc.Results) != 2 || doc.Summary["total"] != 2 {
		t.Errorf("doc = %+v", doc)
	}
}

func TestExitCodeConfigError(t *testing.T) {
	if got := ExitCode(grouped()); got != 2 {
		t.Errorf("a config that could not run must give 2, got %d", got)
	}
}

func TestTextAll(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "text", false, false, one(sample())); err != nil {
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
	if err := Write(&b, "text", true, false, one(sample())); err != nil {
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
	if err := Write(&b, "json", true, false, one(sample())); err != nil {
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
	if err := Write(&b, "json", false, false, one(nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"results": []`) {
		t.Errorf("empty results must be [], got %s", b.String())
	}
}

func TestUnknownFormat(t *testing.T) {
	if err := Write(&bytes.Buffer{}, "xml", false, false, nil); err == nil {
		t.Error("unknown format must be an error")
	}
}

func TestExitCode(t *testing.T) {
	all := sample()
	if ExitCode(one(all[:1])) != 0 || ExitCode(one(nil)) != 0 {
		t.Error("all passing (or nothing to run) must be 0")
	}
	if ExitCode(one(all[:2])) != 1 || ExitCode(one(all[3:])) != 1 {
		t.Error("any fail or error must be 1")
	}
	if ExitCode(one(all[2:3])) != 1 {
		t.Error("the exit code follows the raw status of this run, not the confirmed state")
	}
}
