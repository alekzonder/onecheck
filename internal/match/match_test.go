package match

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/alekzonder/onecheck/pkg/module"
)

var testTypes = FieldTypes{
	"status":  module.TypeNumber,
	"body":    module.TypeString,
	"headers": module.TypeObject,
	"size":    module.TypeBytes,
	"age":     module.TypeDuration,
	"exists":  module.TypeBool,
	"items":   module.TypeArray,
	"nothing": module.TypeString,
}

const testFields = `{
  "status": 200,
  "body": "{\"status\":\"degraded\",\"items\":[{\"id\":1,\"state\":\"ok\"},{\"id\":2,\"state\":\"bad\"}],\"n\":null}",
  "headers": {"content-type": "application/json; charset=utf-8", "x-count": "3"},
  "size": 2048,
  "age": 120.5,
  "exists": true,
  "items": ["a", "b", 3]
}`

func compile(t *testing.T, checkYAML string) (*Check, error) {
	t.Helper()
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(checkYAML), &raw); err != nil {
		t.Fatalf("bad test yaml: %v", err)
	}
	return Compile(raw, testTypes)
}

func eval(t *testing.T, checkYAML string) []Failure {
	t.Helper()
	c, err := compile(t, checkYAML)
	if err != nil {
		t.Fatalf("compile %q: %v", checkYAML, err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(testFields), &fields); err != nil {
		t.Fatal(err)
	}
	failures, err := c.Eval(fields, nil)
	if err != nil {
		t.Fatalf("eval %q: %v", checkYAML, err)
	}
	return failures
}

func TestOperators(t *testing.T) {
	cases := []struct {
		name  string
		check string
		pass  bool
	}{
		{"equals number", `status: {equals: 200}`, true},
		{"equals number fail", `status: {equals: 201}`, false},
		{"equals no coercion", `status: {equals: "200"}`, false},
		{"equals bool", `exists: {equals: true}`, true},
		{"equals array", `items: {equals: [a, b, 3]}`, true},
		{"not_equals", `status: {not_equals: 500}`, true},
		{"not_equals fail", `status: {not_equals: 200}`, false},
		{"contains substring", `body: {contains: degraded}`, true},
		{"contains substring fail", `body: {contains: healthy}`, false},
		{"contains array element", `items: {contains: 3}`, true},
		{"contains array element fail", `items: {contains: c}`, false},
		{"contains object key", `headers: {contains: x-count}`, true},
		{"contains object key fail", `headers: {contains: x-missing}`, false},
		{"contains wrong type", `status: {contains: "2"}`, false},
		{"not_contains", `body: {not_contains: healthy}`, true},
		{"not_contains fail", `body: {not_contains: degraded}`, false},
		{"starts_with", `headers.content-type: {starts_with: application/}`, true},
		{"starts_with fail", `headers.content-type: {starts_with: text/}`, false},
		{"ends_with", `headers.content-type: {ends_with: utf-8}`, true},
		{"ends_with fail", `headers.content-type: {ends_with: json}`, false},
		{"matches", `headers.x-count: {matches: '^\d+$'}`, true},
		{"matches fail", `headers.x-count: {matches: '^[a-z]+$'}`, false},
		{"matches wrong type", `status: {matches: '200'}`, false},
		{"not_matches", `body: {not_matches: 'ERROR'}`, true},
		{"not_matches fail", `body: {not_matches: 'degr.ded'}`, false},
		{"gt", `status: {gt: 199}`, true},
		{"gt fail", `status: {gt: 200}`, false},
		{"gte", `status: {gte: 200}`, true},
		{"gte fail", `status: {gte: 201}`, false},
		{"lt", `status: {lt: 300}`, true},
		{"lt fail", `status: {lt: 200}`, false},
		{"lte", `status: {lte: 200}`, true},
		{"lte fail", `status: {lte: 199}`, false},
		{"gt wrong type", `body: {gt: 1}`, false},
		{"in", `status: {in: [200, 204]}`, true},
		{"in fail", `status: {in: [301, 302]}`, false},
		{"not_in", `status: {not_in: [500, 502]}`, true},
		{"not_in fail", `status: {not_in: [200]}`, false},
		{"exists true", `headers.x-count: {exists: true}`, true},
		{"exists true fail", `headers.x-none: {exists: true}`, false},
		{"exists false", `headers.x-none: {exists: false}`, true},
		{"exists false fail", `headers.x-count: {exists: false}`, false},
		{"type", `headers: {type: object}`, true},
		{"type array", `items: {type: array}`, true},
		{"type fail", `status: {type: string}`, false},
		{"length string", `headers.x-count: {length: 1}`, true},
		{"length array", `items: {length: {gte: 3, lt: 10}}`, true},
		{"length object", `headers: {length: 2}`, true},
		{"length fail", `items: {length: {gt: 3}}`, false},
		{"length wrong type", `status: {length: 3}`, false},
		{"glob", `headers.content-type: {glob: "application/*"}`, true},
		{"glob whole string", `headers.content-type: {glob: "json*"}`, false},
		{"glob wrong type", `status: {glob: "2*"}`, false},
		{"not_glob", `headers.content-type: {not_glob: "text/*"}`, true},
		{"not_glob fail", `headers.content-type: {not_glob: "*utf-8"}`, false},
		{"and of operators", `status: {gte: 200, lt: 300}`, true},
		{"and of operators fail", `status: {gte: 200, lt: 200}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			failures := eval(t, c.check)
			if c.pass && len(failures) != 0 {
				t.Errorf("expected pass, got %+v", failures)
			}
			if !c.pass && len(failures) == 0 {
				t.Errorf("expected a failure")
			}
		})
	}
}

func TestShorthandEquals(t *testing.T) {
	if f := eval(t, `status: 200`); len(f) != 0 {
		t.Errorf("unexpected failures: %+v", f)
	}
	f := eval(t, `exists: false`)
	if len(f) != 1 || f[0].Operator != "equals" || f[0].Expected != false || f[0].Actual != true {
		t.Errorf("failures = %+v", f)
	}
}

func TestCombinators(t *testing.T) {
	cases := []struct {
		check string
		pass  bool
	}{
		{`status: {any: [{equals: 500}, {lt: 300}]}`, true},
		{`status: {any: [{equals: 500}, {gt: 300}]}`, false},
		{`status: {all: [{gte: 200}, {lt: 300}]}`, true},
		{`status: {all: [{gte: 200}, {lt: 100}]}`, false},
		{`status: {not: {in: [500, 502]}}`, true},
		{`status: {not: {equals: 200}}`, false},
		{`status: {not: {any: [{lt: 100}, {gt: 500}]}}`, true},
		{`body: {any: [{contains: healthy}, {all: [{contains: degraded}, {not: {contains: down}}]}]}`, true},
	}
	for _, c := range cases {
		failures := eval(t, c.check)
		if c.pass != (len(failures) == 0) {
			t.Errorf("%s: pass=%v, failures=%+v", c.check, c.pass, failures)
		}
	}
	f := eval(t, `status: {any: [{equals: 500}, {gt: 300}]}`)
	if len(f) != 1 || f[0].Operator != "any" {
		t.Errorf("any must produce exactly one failure, got %+v", f)
	}
}

func TestMissingPath(t *testing.T) {
	failing := []string{
		`headers.x-none: {equals: a}`,
		`headers.x-none: {not_equals: a}`,
		`headers.x-none: {not_contains: a}`,
		`headers.x-none: {not_in: [a]}`,
		`headers.x-none: {not: {equals: a}}`,
		`nothing: {type: string}`,
	}
	for _, c := range failing {
		f := eval(t, c)
		if len(f) == 0 {
			t.Errorf("%s: a missing path must fail", c)
			continue
		}
		if !strings.Contains(f[0].Message, "path is missing") {
			t.Errorf("%s: message = %q", c, f[0].Message)
		}
	}
	passing := []string{
		`headers.x-none: {exists: false}`,
		`headers.x-none: {not: {exists: true}}`,
	}
	for _, c := range passing {
		if f := eval(t, c); len(f) != 0 {
			t.Errorf("%s: unexpected failures %+v", c, f)
		}
	}
}

func TestTypedUnits(t *testing.T) {
	cases := []struct {
		check string
		pass  bool
	}{
		{`size: {lt: 10MB}`, true},
		{`size: {gt: 1KB, lte: 2KB}`, true},
		{`size: {lt: 1KB}`, false},
		{`size: 2KB`, true},
		{`age: {lt: 1h}`, true},
		{`age: {gt: 3m}`, false},
		{`age: {gte: 120}`, true},
	}
	for _, c := range cases {
		failures := eval(t, c.check)
		if c.pass != (len(failures) == 0) {
			t.Errorf("%s: pass=%v, failures=%+v", c.check, c.pass, failures)
		}
	}
}

func TestJSONAssert(t *testing.T) {
	ok := `
body:
  json:
    assert:
      - {path: items.#, equals: 2}
      - {path: items.0.id, type: number}
      - {path: 'items.#(state=="bad")#', length: 1}
      - {path: items.1.state, in: [bad, worse]}
      - {path: n, type: "null"}
      - {path: missing, exists: false}
`
	if f := eval(t, ok); len(f) != 0 {
		t.Errorf("unexpected failures: %+v", f)
	}
	f := eval(t, `body: {json: {assert: [{path: status, equals: ok}, {path: items.#, gt: 5}]}}`)
	if len(f) != 2 {
		t.Fatalf("want 2 failures, got %+v", f)
	}
	if f[0].Path != "body" || f[0].JSONPath != "status" || f[0].Operator != "equals" || f[0].Actual != "degraded" {
		t.Errorf("failure = %+v", f[0])
	}
}

func TestJSONSchema(t *testing.T) {
	ok := `
body:
  json:
    schema:
      type: object
      required: [status, items]
      properties:
        items: {type: array, minItems: 1}
`
	if f := eval(t, ok); len(f) != 0 {
		t.Errorf("unexpected failures: %+v", f)
	}
	bad := `
body:
  json:
    schema:
      type: object
      required: [version]
      properties:
        status: {enum: [ok]}
`
	f := eval(t, bad)
	if len(f) != 2 {
		t.Fatalf("want 2 schema failures, got %+v", f)
	}
	for _, x := range f {
		if x.Operator != "schema" || x.Path != "body" {
			t.Errorf("failure = %+v", x)
		}
	}
}

func TestJSONInvalid(t *testing.T) {
	f := eval(t, `headers.content-type: {json: {assert: [{path: a, exists: true}]}}`)
	if len(f) != 1 || f[0].Operator != "json" || !strings.Contains(f[0].Message, "not valid JSON") {
		t.Errorf("failures = %+v", f)
	}
	f = eval(t, `status: {json: {assert: [{path: a, exists: true}]}}`)
	if len(f) != 1 || f[0].Operator != "json" {
		t.Errorf("failures = %+v", f)
	}
}

func TestCompileErrors(t *testing.T) {
	cases := map[string]string{
		"unknown operator":     `status: {equal: 200}`,
		"unknown field":        `stauts: 200`,
		"bad regex":            `body: {matches: '('}`,
		"string arg on number": `status: {gt: 10MB}`,
		"bad unit":             `size: {gt: 10XB}`,
		"in needs list":        `status: {in: 200}`,
		"exists needs bool":    `status: {exists: "yes"}`,
		"bad type name":        `status: {type: integer}`,
		"any needs list":       `status: {any: {equals: 1}}`,
		"empty any":            `status: {any: []}`,
		"json needs map":       `body: {json: true}`,
		"json empty":           `body: {json: {}}`,
		"json unknown key":     `body: {json: {asserts: []}}`,
		"assert without path":  `body: {json: {assert: [{equals: 1}]}}`,
		"bad schema":           `body: {json: {schema: {type: 12}}}`,
		"starts_with number":   `body: {starts_with: 1}`,
		"empty condition":      `status: {}`,
		"glob needs string":    `body: {glob: 1}`,
		"bad glob":             `body: {glob: "[a"}`,
	}
	for name, check := range cases {
		if _, err := compile(t, check); err == nil {
			t.Errorf("%s: %s must not compile", name, check)
		}
	}
	_, err := compile(t, `stauts: 200`)
	if err == nil || !strings.Contains(err.Error(), "status") {
		t.Errorf("unknown field error must list known fields, got %v", err)
	}
}

func TestFields(t *testing.T) {
	c, err := compile(t, "status: 200\nheaders.content-type: {contains: json}\nheaders.x-count: '3'\nbody: {contains: a}")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(c.Fields(), ",")
	if got != "body,headers,status" {
		t.Errorf("Fields() = %s", got)
	}
}

func TestEvalTruncatedField(t *testing.T) {
	c, err := compile(t, "body: {contains: a}\nstatus: 200")
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{"body": "aaa", "status": 200}
	if _, err := c.Eval(fields, []string{"body"}); err == nil || !strings.Contains(err.Error(), "body") {
		t.Errorf("a condition on a truncated field must be an error, got %v", err)
	}
	if _, err := c.Eval(fields, []string{"other"}); err != nil {
		t.Errorf("unused truncated field must not matter: %v", err)
	}
}

func TestFailureMessage(t *testing.T) {
	f := eval(t, `body: {json: {assert: [{path: status, equals: ok}]}}`)
	want := `body json status: expected equals "ok", got "degraded"`
	if len(f) != 1 || f[0].Message != want {
		t.Errorf("message = %+v, want %q", f, want)
	}
	f = eval(t, `status: {in: [301, 302]}`)
	want = `status: expected in [301,302], got 200`
	if len(f) != 1 || f[0].Message != want {
		t.Errorf("message = %+v, want %q", f, want)
	}
	f = eval(t, `headers.x-none: abc`)
	want = `headers.x-none: expected equals "abc", got nothing (path is missing)`
	if len(f) != 1 || f[0].Message != want {
		t.Errorf("message = %+v, want %q", f, want)
	}
	f = eval(t, `items: {length: {gt: 3}}`)
	want = `items: expected length gt 3, got 3`
	if len(f) != 1 || f[0].Message != want {
		t.Errorf("message = %+v, want %q", f, want)
	}
}

func TestActualIsCut(t *testing.T) {
	c, err := compile(t, `body: {equals: x}`)
	if err != nil {
		t.Fatal(err)
	}
	f, err := c.Eval(map[string]any{"body": strings.Repeat("a", 5000)}, nil)
	if err != nil || len(f) != 1 {
		t.Fatalf("f=%v err=%v", f, err)
	}
	if s, _ := f[0].Actual.(string); len(s) > 1100 {
		t.Errorf("actual is %d bytes, want it cut to about 1 KB", len(s))
	}
}

func TestEmptyCheck(t *testing.T) {
	c, err := Compile(nil, testTypes)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := c.Eval(map[string]any{}, nil); err != nil || len(f) != 0 {
		t.Errorf("empty check must pass: %v %v", f, err)
	}
}

const containerFields = `{
  "c": [
    {"name": "nginx", "image": "nginx:1.27", "state": "running", "restarts": 0},
    {"name": "miner", "image": "xmrig/xmrig", "state": "running", "restarts": 7},
    {"name": "old", "image": "registry.local/old:1", "state": "exited", "restarts": 0}
  ],
  "e": [],
  "s": "text"
}`

func evalOn(t *testing.T, fieldsJSON, checkYAML string) []Failure {
	t.Helper()
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(checkYAML), &raw); err != nil {
		t.Fatalf("bad test yaml: %v", err)
	}
	c, err := Compile(raw, nil)
	if err != nil {
		t.Fatalf("compile %q: %v", checkYAML, err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		t.Fatal(err)
	}
	failures, err := c.Eval(fields, nil)
	if err != nil {
		t.Fatalf("eval %q: %v", checkYAML, err)
	}
	return failures
}

func TestArrayConditions(t *testing.T) {
	cases := []struct {
		name  string
		check string
		fails int
	}{
		{"path", `c.0: {path: state, equals: running}`, 0},
		{"path fail", `c.2: {path: state, equals: running}`, 1},
		{"path missing", `c.0: {path: nope, equals: 1}`, 1},
		{"path exists false", `c.0: {path: nope, exists: false}`, 0},
		{"every", `c: {every: {path: name, exists: true}}`, 0},
		{"every fail per element", `c: {every: {path: state, equals: running}}`, 1},
		{"every list is all", `c: {every: [{path: state, equals: running}, {path: restarts, lt: 5}]}`, 2},
		{"every scalar shorthand", `c.#.state: {every: running}`, 1},
		{"every empty array", `e: {every: {equals: 1}}`, 0},
		{"every filtered empty", `c.#(state=="paused")#: {every: {equals: 1}}`, 0},
		{"every not array", `s: {every: {equals: 1}}`, 1},
		{"every missing", `nope: {every: {equals: 1}}`, 1},
		{"some", `c: {some: {path: name, equals: nginx}}`, 0},
		{"some glob", `c.#.image: {some: {glob: "registry.local/*"}}`, 0},
		{"some fail", `c: {some: {path: name, equals: redis}}`, 1},
		{"some empty array", `e: {some: {equals: 1}}`, 1},
		{"not some", `c: {not: {some: {path: name, equals: miner}}}`, 1},
		{"strangers", `c.#(state=="running")#: {every: {any: [{path: name, in: [nginx]}, {path: image, glob: "registry.local/*"}]}}`, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := evalOn(t, containerFields, c.check); len(got) != c.fails {
				t.Errorf("%d failures, want %d: %+v", len(got), c.fails, got)
			}
		})
	}
}

func TestEveryFailureNamesElement(t *testing.T) {
	got := evalOn(t, containerFields, `c: {every: {path: state, equals: running}}`)
	if len(got) != 1 || got[0].Path != "c.2.state" || !strings.HasPrefix(got[0].Message, `c.2.state: expected equals "running", got "exited"`) {
		t.Errorf("failures = %+v", got)
	}
}

func TestEveryAnyCarriesElement(t *testing.T) {
	got := evalOn(t, containerFields,
		`c.#(state=="running")#: {every: {any: [{path: name, in: [nginx]}, {path: image, glob: "registry.local/*"}]}}`)
	if len(got) != 1 || !strings.Contains(got[0].Message, `"name":"miner"`) || !strings.Contains(got[0].Message, `.1:`) {
		t.Errorf("failures = %+v", got)
	}
}

func TestSomeFailureMessage(t *testing.T) {
	got := evalOn(t, containerFields, `c.#.name: {some: {equals: redis}}`)
	if len(got) != 1 || !strings.Contains(got[0].Message, "expected some") || !strings.Contains(got[0].Message, "miner") {
		t.Errorf("failures = %+v", got)
	}
}

func TestArrayConditionCompileErrors(t *testing.T) {
	for _, check := range []string{
		`items: {path: x}`,
		`items: {path: 1, equals: 2}`,
		`items: {every: []}`,
		`items: {some: {}}`,
		`items: {every: {path: x, equal: 1}}`,
	} {
		if _, err := compile(t, check); err == nil {
			t.Errorf("%s must not compile", check)
		}
	}
}

func TestAnyOnElementShowsReferencedKeys(t *testing.T) {
	fields := `{"c": [{"name": "miner", "image": "xmrig", "labels": {"a": "` + strings.Repeat("x", 2000) + `"}}]}`
	got := evalOn(t, fields, `c: {every: {any: [{path: name, in: [nginx]}, {path: image, glob: "registry.local/*"}]}}`)
	if len(got) != 1 || !strings.HasSuffix(got[0].Message, `got {"image":"xmrig","name":"miner"}`) {
		t.Errorf("failures = %+v", got)
	}
}
