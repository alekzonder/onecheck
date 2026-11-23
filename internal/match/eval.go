package match

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
)

// maxActual is the largest "actual" value, in bytes, kept in a Failure.
const maxActual = 1024

// Failure describes one condition that did not hold.
type Failure struct {
	Path     string `json:"path"`
	JSONPath string `json:"json_path,omitempty"`
	Operator string `json:"operator"`
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
	Message  string `json:"message"`
}

// value is a JSON value looked up by path; exists is false when the path is
// absent (which is different from an explicit null).
type value struct {
	v      any
	exists bool
}

// recorder collects failures for one path.
type recorder struct {
	path     string
	jsonPath string
	prefix   string // operator prefix, e.g. "length "
	out      *[]Failure
}

func (r recorder) scratch() recorder {
	r.out = &[]Failure{}
	return r
}

func (r recorder) where() string {
	if r.jsonPath != "" {
		return r.path + " json " + r.jsonPath
	}
	return r.path
}

func (r recorder) fail(op string, expected any, v value, note string) {
	got := "nothing (path is missing)"
	var actual any
	if v.exists {
		actual = cut(v.v)
		got = toJSON(actual)
	}
	msg := fmt.Sprintf("%s: expected %s%s %s, got %s", r.where(), r.prefix, op, toJSON(expected), got)
	if note != "" {
		msg += " (" + note + ")"
	}
	r.add(Failure{Operator: r.prefix + op, Expected: expected, Actual: actual, Message: msg})
}

func (r recorder) add(f Failure) {
	f.Path = r.path
	if f.JSONPath == "" {
		f.JSONPath = r.jsonPath
	}
	*r.out = append(*r.out, f)
}

// node is one compiled condition.
type node interface {
	// eval records failures and reports whether the condition holds.
	eval(r recorder, v value) bool
	// missingAware reports whether the condition gives a meaningful answer
	// for an absent path (only "exists" does).
	missingAware() bool
}

type leaf struct {
	op       string
	expected any
	fn       func(v any) (ok bool, note string)
}

func (n leaf) missingAware() bool { return false }

func (n leaf) eval(r recorder, v value) bool {
	if !v.exists {
		r.fail(n.op, n.expected, v, "")
		return false
	}
	ok, note := n.fn(v.v)
	if !ok {
		r.fail(n.op, n.expected, v, note)
	}
	return ok
}

type existsNode struct{ want bool }

func (n existsNode) missingAware() bool { return true }

func (n existsNode) eval(r recorder, v value) bool {
	if v.exists != n.want {
		r.fail("exists", n.want, v, "")
		return false
	}
	return true
}

type allNode []node

func (n allNode) missingAware() bool {
	for _, c := range n {
		if !c.missingAware() {
			return false
		}
	}
	return true
}

func (n allNode) eval(r recorder, v value) bool {
	ok := true
	for _, c := range n {
		if !c.eval(r, v) {
			ok = false
		}
	}
	return ok
}

type anyNode struct {
	children allNode
	raw      any
}

func (n anyNode) missingAware() bool { return n.children.missingAware() }

func (n anyNode) eval(r recorder, v value) bool {
	for _, c := range n.children {
		if c.eval(r.scratch(), v) {
			return true
		}
	}
	r.fail("any", n.raw, v, "")
	return false
}

type notNode struct {
	inner node
	raw   any
}

func (n notNode) missingAware() bool { return n.inner.missingAware() }

func (n notNode) eval(r recorder, v value) bool {
	// A negated condition on an absent path only holds when the condition
	// itself is about existence; otherwise missing data would pass silently.
	if !v.exists && !n.inner.missingAware() {
		r.fail("not", n.raw, v, "")
		return false
	}
	if n.inner.eval(r.scratch(), v) {
		r.fail("not", n.raw, v, "")
		return false
	}
	return true
}

type lengthNode struct {
	inner node
	raw   any
}

func (n lengthNode) missingAware() bool { return false }

func (n lengthNode) eval(r recorder, v value) bool {
	if !v.exists {
		r.fail("length", n.raw, v, "")
		return false
	}
	var length int
	switch x := v.v.(type) {
	case string:
		length = len([]rune(x))
	case []any:
		length = len(x)
	case map[string]any:
		length = len(x)
	default:
		r.fail("length", n.raw, v, "a "+typeName(v.v)+" has no length")
		return false
	}
	r.prefix += "length "
	return n.inner.eval(r, value{v: float64(length), exists: true})
}

// Eval checks the collected fields against the compiled conditions. It
// returns an error, not failures, when a condition uses a field that the
// module had to truncate: such a result would be unreliable.
func (c *Check) Eval(fields map[string]any, truncated []string) ([]Failure, error) {
	for _, f := range truncated {
		if _, used := c.fields[f]; used {
			return nil, fmt.Errorf("field %q is larger than run.max_field_size and was truncated; cannot check it reliably", f)
		}
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("fields are not valid JSON: %w", err)
	}
	failures := []Failure{}
	for _, e := range c.entries {
		e.node.eval(recorder{path: e.path, out: &failures}, lookup(data, e.path))
	}
	return failures, nil
}

// Fields returns the sorted names of the module fields the conditions use.
func (c *Check) Fields() []string {
	names := make([]string, 0, len(c.fields))
	for f := range c.fields {
		names = append(names, f)
	}
	sort.Strings(names)
	return names
}

func lookup(data []byte, path string) value {
	res := gjson.GetBytes(data, path)
	return value{v: res.Value(), exists: res.Exists()}
}

func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
}

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// cut limits the size of a value stored in a Failure.
func cut(v any) any {
	if s, ok := v.(string); ok {
		if len(s) > maxActual {
			return strings.ToValidUTF8(s[:maxActual], "") + "…"
		}
		return s
	}
	if _, scalar := v.(float64); scalar || v == nil {
		return v
	}
	if _, isBool := v.(bool); isBool {
		return v
	}
	if s := toJSON(v); len(s) > maxActual {
		return strings.ToValidUTF8(s[:maxActual], "") + "…"
	}
	return v
}
