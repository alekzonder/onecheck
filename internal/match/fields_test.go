package match

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func wantFields(t *testing.T, got []Failure, want ...[]Field) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d failures, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i].Fields, want[i]) {
			t.Errorf("failure %d fields:\n got %+v\nwant %+v", i, got[i].Fields, want[i])
		}
	}
}

func TestFieldsLeaf(t *testing.T) {
	wantFields(t, eval(t, `status: 201`),
		[]Field{{Path: "status", Operator: "equals", Actual: 200.0}})
}

func TestFieldsLengthLeaf(t *testing.T) {
	wantFields(t, eval(t, `items: {length: {gt: 5}}`),
		[]Field{{Path: "items", Operator: "length gt", Actual: 3.0}})
}

func TestFieldsMissing(t *testing.T) {
	wantFields(t, eval(t, `nothing: {equals: x}`),
		[]Field{{Path: "nothing", Operator: "equals", Missing: true}})
}

const dockerFields = `{"containers": [
  {"name": "bot-1", "state": "running", "image": "a"},
  {"name": "postgres-1", "state": "running", "image": "b"},
  {"name": "smoke-tls-proxy", "state": "running", "image": "nginx"},
  {"name": "stopped", "state": "exited", "image": "c"},
  {"name": "smoke-tls-release", "state": "running", "image": "docker"}
]}`

func TestFieldsEveryAny(t *testing.T) {
	got := evalOn(t, dockerFields,
		`containers.#(state=="running")#: {every: {any: [{path: name, in: [bot-1, postgres-1]}]}}`)
	p := `containers.#(state=="running")#.`
	wantFields(t, got,
		[]Field{{Path: p + "2.name", Operator: "in", Actual: "smoke-tls-proxy"}},
		[]Field{{Path: p + "3.name", Operator: "in", Actual: "smoke-tls-release"}})
	if !strings.HasPrefix(got[0].Message, p+`2: expected any [{"in":["bot-1","postgres-1"],"path":"name"}], got {`) {
		t.Errorf("message changed: %s", got[0].Message)
	}
}

func TestFieldsAnyBranches(t *testing.T) {
	got := evalOn(t, containerFields,
		`c.#(state=="running")#: {every: {any: [{path: name, in: [nginx]}, {path: image, glob: "registry.local/*"}, {path: name, in: [web]}]}}`)
	p := `c.#(state=="running")#.1.`
	wantFields(t, got, []Field{
		{Path: p + "name", Operator: "in", Actual: "miner"},
		{Path: p + "image", Operator: "glob", Actual: "xmrig/xmrig"},
	})
}

func TestFieldsNot(t *testing.T) {
	fields := `{"c": [{"name": "a"}, {"name": "b", "health": "unhealthy"}]}`
	wantFields(t, evalOn(t, fields, `c.1: {not: {path: health, equals: unhealthy}}`),
		[]Field{{Path: "c.1.health", Operator: "equals", Actual: "unhealthy"}})
	wantFields(t, evalOn(t, fields, `c.0: {not: {path: health, equals: unhealthy}}`),
		[]Field{{Path: "c.0.health", Operator: "equals", Missing: true}})
	wantFields(t, evalOn(t, fields, `nope: {not: {equals: 1}}`),
		[]Field{{Path: "nope", Operator: "not", Missing: true}})
}

func TestFieldsNotAny(t *testing.T) {
	fields := `{"x": 1, "y": 2}`
	wantFields(t, evalOn(t, fields, `x: {not: {any: [{equals: 5}, {equals: 1}]}}`),
		[]Field{{Path: "x", Operator: "equals", Actual: 1.0}})
}

func TestFieldsSome(t *testing.T) {
	wantFields(t, evalOn(t, containerFields, `c: {some: {path: name, equals: redis}}`), []Field{
		{Path: "c.0.name", Operator: "equals", Actual: "nginx"},
		{Path: "c.1.name", Operator: "equals", Actual: "miner"},
		{Path: "c.2.name", Operator: "equals", Actual: "old"},
	})
}

func TestFieldsSomeCapped(t *testing.T) {
	items := make([]string, 30)
	for i := range items {
		items[i] = fmt.Sprintf(`{"n": %d}`, i)
	}
	got := evalOn(t, `{"a": [`+strings.Join(items, ",")+`]}`, `a: {some: {path: n, equals: 100}}`)
	if len(got) != 1 || len(got[0].Fields) != maxFields || got[0].Fields[19].Path != "a.19.n" {
		t.Errorf("failures = %+v", got)
	}
}

func TestFieldsArrayMissing(t *testing.T) {
	wantFields(t, evalOn(t, containerFields, `nope: {every: {equals: 1}}`),
		[]Field{{Path: "nope", Operator: "every", Missing: true}})
}

func TestFieldsJSONAssertUnderAny(t *testing.T) {
	wantFields(t, eval(t, `body: {any: [{json: {assert: [{path: status, equals: ok}]}}]}`),
		[]Field{{Path: "body", JSONPath: "status", Operator: "equals", Actual: "degraded"}})
}

func TestFailureFieldsMarshalEmpty(t *testing.T) {
	b, err := json.Marshal(Failure{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"fields":[]`) {
		t.Errorf("got %s", b)
	}
	b, _ = json.Marshal(Field{Path: "x", Operator: "equals"})
	if string(b) != `{"path":"x","operator":"equals","actual":null}` {
		t.Errorf("field json = %s", b)
	}
}
