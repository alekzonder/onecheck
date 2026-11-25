package match

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
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
	// Fields lists the concrete leaf fields behind the failure.
	Fields []Field `json:"fields"`
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
	held     *[]Field // leaves that held; collected only inside "not"
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
	r.failFields(op, expected, v, note, nil)
}

// failFields records a failure whose fields are the given leaves, or the
// value v itself when there are none.
func (r recorder) failFields(op string, expected any, v value, note string, fields []Field) {
	if fields == nil {
		fields = []Field{r.leafField(op, v)}
	}
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
	r.add(Failure{Operator: r.prefix + op, Expected: expected, Actual: actual, Message: msg, Fields: fields})
}

func (r recorder) add(f Failure) {
	f.Path = r.path
	if f.JSONPath == "" {
		f.JSONPath = r.jsonPath
	}
	if f.Fields == nil {
		f.Fields = []Field{{Path: r.path, JSONPath: f.JSONPath, Operator: f.Operator, Actual: f.Actual}}
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
		return false
	}
	r.hold(n.op, v)
	return true
}

type existsNode struct{ want bool }

func (n existsNode) missingAware() bool { return true }

func (n existsNode) eval(r recorder, v value) bool {
	if v.exists != n.want {
		r.fail("exists", n.want, v, "")
		return false
	}
	r.hold("exists", v)
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
	var fields []Field
	for _, c := range n.children {
		s := r.scratch()
		if c.eval(s, v) {
			return true
		}
		fields = append(fields, fieldsOf(*s.out)...)
	}
	r.failFields("any", n.raw, focus(v, n.children), "", merge(fields))
	return false
}

// focus narrows an object shown in a failure to the keys the condition
// looks at through "path", so a large element (e.g. one with many labels)
// still shows what decided the result.
func focus(v value, n node) value {
	obj, ok := v.v.(map[string]any)
	if !ok {
		return v
	}
	keys := map[string]struct{}{}
	referencedKeys(n, keys)
	out := scalars(obj)
	found := false
	for k := range keys {
		if val, has := obj[k]; has {
			out[k] = val
			found = true
		}
	}
	if !found {
		return v
	}
	return value{v: out, exists: true}
}

// scalars returns the keys of an object whose values are not objects or
// arrays: the element's identity (name, id, ...) without bulky parts.
func scalars(obj map[string]any) map[string]any {
	out := map[string]any{}
	for k, val := range obj {
		switch val.(type) {
		case map[string]any, []any:
		default:
			out[k] = val
		}
	}
	return out
}

func referencedKeys(n node, keys map[string]struct{}) {
	switch x := n.(type) {
	case pathNode:
		keys[topField(x.path)] = struct{}{}
	case allNode:
		for _, c := range x {
			referencedKeys(c, keys)
		}
	case anyNode:
		referencedKeys(x.children, keys)
	case notNode:
		referencedKeys(x.inner, keys)
	}
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
	held := []Field{}
	s := r.scratch()
	s.held = &held
	missing := v.exists && missingIn(n.inner, v)
	if !n.inner.eval(s, v) && !missing {
		return true
	}
	r.failFields("not", n.raw, v, "", merge(append(held, fieldsOf(*s.out)...)))
	return false
}

// missingIn reports whether a condition looks through "path" at a value
// that is absent, where it is not about existence: negating it would let
// missing data pass.
func missingIn(n node, v value) bool {
	switch x := n.(type) {
	case pathNode:
		sub := x.lookup(v)
		if !sub.exists {
			return !x.inner.missingAware()
		}
		return missingIn(x.inner, sub)
	case allNode:
		for _, c := range x {
			if missingIn(c, v) {
				return true
			}
		}
	case anyNode:
		return missingIn(x.children, v)
	case notNode:
		return missingIn(x.inner, v)
	}
	return false
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

// child returns a recorder for a value inside the current one: an element
// index or a sub-path.
func (r recorder) child(seg string) recorder {
	if r.jsonPath != "" {
		r.jsonPath += "." + seg
	} else {
		r.path += "." + seg
	}
	r.prefix = ""
	return r
}

// pathNode applies a condition to the value at a path inside the current
// value.
type pathNode struct {
	path  string
	inner node
}

func (n pathNode) missingAware() bool { return n.inner.missingAware() }

func (n pathNode) lookup(v value) value {
	if !v.exists {
		return value{}
	}
	data, err := json.Marshal(v.v)
	if err != nil {
		return value{}
	}
	return lookup(data, n.path)
}

func (n pathNode) eval(r recorder, v value) bool {
	return n.inner.eval(r.child(n.path), n.lookup(v))
}

// elements returns the array a condition on elements applies to, or records
// why there is none.
func elements(r recorder, op string, raw any, v value) ([]any, bool) {
	if !v.exists {
		r.fail(op, raw, v, "")
		return nil, false
	}
	arr, ok := v.v.([]any)
	if !ok {
		r.fail(op, raw, v, "not applicable to a "+typeName(v.v))
	}
	return arr, ok
}

// everyNode holds when its condition holds for each element of an array;
// every failing element reports its own failures.
type everyNode struct {
	inner node
	raw   any
}

func (n everyNode) missingAware() bool { return false }

func (n everyNode) eval(r recorder, v value) bool {
	arr, ok := elements(r, "every", n.raw, v)
	if !ok {
		return false
	}
	for i, el := range arr {
		start := len(*r.out)
		if !n.inner.eval(r.child(strconv.Itoa(i)), value{v: el, exists: true}) {
			ok = false
			identify(*r.out, start, el)
		}
	}
	return ok
}

// identify appends the element's scalar keys to failures recorded for it,
// so a message about "containers.1.state" also says which container that
// is. Failures of "any" already show the element.
func identify(failures []Failure, start int, el any) {
	obj, isObj := el.(map[string]any)
	if !isObj {
		return
	}
	id := scalars(obj)
	if len(id) == 0 {
		return
	}
	note := " (element " + toJSON(cut(id)) + ")"
	for j := start; j < len(failures); j++ {
		if failures[j].Operator != "any" {
			failures[j].Message += note
		}
	}
}

// someNode holds when its condition holds for at least one element.
type someNode struct {
	inner node
	raw   any
}

func (n someNode) missingAware() bool { return false }

func (n someNode) eval(r recorder, v value) bool {
	arr, ok := elements(r, "some", n.raw, v)
	if !ok {
		return false
	}
	var fields []Field
	for i, el := range arr {
		s := r.child(strconv.Itoa(i)).scratch()
		if n.inner.eval(s, value{v: el, exists: true}) {
			return true
		}
		fields = append(fields, fieldsOf(*s.out)...)
	}
	r.failFields("some", n.raw, v, "", merge(fields))
	return false
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
