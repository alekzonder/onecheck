// Package match is the single matching engine of onecheck: it compiles the
// "check" block of a rule and evaluates it against field values collected by
// a module.
package match

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alekzonder/onecheck/pkg/module"
)

// FieldTypes maps the fields a module can return to their types.
type FieldTypes map[string]module.FieldType

// ElementTypes maps an array field to the types of the keys of its elements.
type ElementTypes map[string]FieldTypes

// typing is what compilation knows about the value a condition applies to.
// Only bytes and duration matter: they let a condition use unit literals.
type typing struct {
	// value is the type of the value itself.
	value module.FieldType
	// keys are the types of the keys of the value, when it is an element
	// of a typed array.
	keys FieldTypes
	// elems are the types of the keys of the value's elements, when it is
	// a typed array.
	elems FieldTypes
}

// Check is a compiled "check" block.
type Check struct {
	entries []entry
	fields  map[string]struct{}
}

type entry struct {
	path string
	node node
}

// Compile validates and compiles a "check" block: a mapping from a path into
// the module fields to a condition. With non-nil types, every path must start
// with a known field. elements gives the key types of the elements of array
// fields, so that conditions on those keys can use unit literals too.
func Compile(raw map[string]any, types FieldTypes, elements ElementTypes) (*Check, error) {
	c := &Check{fields: map[string]struct{}{}}
	norm, err := normalize(raw)
	if err != nil {
		return nil, fmt.Errorf("check: %w", err)
	}
	conds, _ := norm.(map[string]any)
	paths := make([]string, 0, len(conds))
	for p := range conds {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, path := range paths {
		field := topField(path)
		ft, known := types[field]
		if types != nil && !known {
			return nil, fmt.Errorf("check %q: the module has no field %q (known fields: %s)", path, field, knownFields(types))
		}
		n, err := compileCond(conds[path], topTyping(path, field, ft, elements[field]))
		if err != nil {
			return nil, fmt.Errorf("check %q: %w", path, err)
		}
		c.entries = append(c.entries, entry{path: path, node: n})
		c.fields[field] = struct{}{}
	}
	return c, nil
}

// normalize converts YAML-decoded values into plain JSON values so that all
// numbers are float64 and all maps are map[string]any.
func normalize(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("conditions must be plain data with string keys: %w", err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// topTyping tells what a check key addresses: the field itself, one element
// of a typed array (field.0, field.#(query)), a list of its elements
// (field.#(query)#) or a typed key of one element (field.0.key). Any other
// path is untyped: unit literals do not apply there.
func topTyping(path, field string, ft module.FieldType, elems FieldTypes) typing {
	if path == field {
		return typing{value: ft, elems: elems}
	}
	rest, ok := strings.CutPrefix(path[len(field):], ".")
	if !ok || elems == nil {
		return typing{}
	}
	switch {
	case oneElement(rest):
		return typing{keys: elems}
	case strings.HasPrefix(rest, "#(") && strings.HasSuffix(rest, ")#") && !strings.Contains(rest, ")."):
		return typing{elems: elems}
	}
	if i := strings.LastIndex(rest, "."); i > 0 && oneElement(rest[:i]) {
		return typing{value: elems[rest[i+1:]]}
	}
	return typing{}
}

// oneElement reports whether a path segment picks a single element of an
// array: an index, or a query for the first match.
func oneElement(seg string) bool {
	if strings.HasPrefix(seg, "#(") {
		return strings.HasSuffix(seg, ")") && !strings.Contains(seg, ").")
	}
	if seg == "" {
		return false
	}
	for _, c := range seg {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func topField(path string) string {
	if i := strings.IndexAny(path, ".|#@"); i >= 0 {
		return path[:i]
	}
	return path
}

func knownFields(types FieldTypes) string {
	names := make([]string, 0, len(types))
	for n := range types {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// compileCond compiles one condition: a scalar (shorthand for equals) or a
// mapping of operators that must all hold.
func compileCond(raw any, t typing) (node, error) {
	ops, isMap := raw.(map[string]any)
	if !isMap {
		return compileOp("equals", raw, t)
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("empty condition")
	}
	if p, has := ops["path"]; has {
		return compilePath(p, ops, t)
	}
	names := make([]string, 0, len(ops))
	for name := range ops {
		names = append(names, name)
	}
	sort.Strings(names)
	nodes := make(allNode, 0, len(names))
	for _, name := range names {
		n, err := compileOp(name, ops[name], t)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	if len(nodes) == 1 {
		return nodes[0], nil
	}
	return nodes, nil
}

// compilePath compiles a condition mapping with a "path" key: the other
// operators apply to the value at that path inside the current value. A path
// that is a typed key of an element gives its value that type.
func compilePath(p any, ops map[string]any, t typing) (node, error) {
	path, ok := p.(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path: needs a non-empty string, got %s", toJSON(p))
	}
	rest := make(map[string]any, len(ops)-1)
	for k, v := range ops {
		if k != "path" {
			rest[k] = v
		}
	}
	if len(rest) == 0 {
		return nil, fmt.Errorf("path %q: needs at least one operator next to it", path)
	}
	inner, err := compileCond(rest, typing{value: t.keys[path]})
	if err != nil {
		return nil, fmt.Errorf("path %q: %w", path, err)
	}
	return pathNode{path: path, inner: inner}, nil
}

// compileElem compiles the argument of every/some: a condition, or a list
// of conditions that must all hold. Each applies to one element of the array.
func compileElem(name string, arg any, t typing) (node, error) {
	elem := typing{keys: t.elems}
	if _, isList := arg.([]any); isList {
		return compileList(name, arg, elem)
	}
	inner, err := compileCond(arg, elem)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return inner, nil
}

func compileOp(name string, arg any, t typing) (node, error) {
	switch name {
	case "every", "some":
		inner, err := compileElem(name, arg, t)
		if err != nil {
			return nil, err
		}
		if name == "some" {
			return someNode{inner: inner, raw: arg}, nil
		}
		return everyNode{inner: inner, raw: arg}, nil
	case "all", "any":
		children, err := compileList(name, arg, t)
		if err != nil {
			return nil, err
		}
		if name == "any" {
			return anyNode{children: children, raw: arg}, nil
		}
		return children, nil
	case "not":
		inner, err := compileCond(arg, t)
		if err != nil {
			return nil, fmt.Errorf("not: %w", err)
		}
		return notNode{inner: inner, raw: arg}, nil
	case "length":
		inner, err := compileCond(arg, typing{})
		if err != nil {
			return nil, fmt.Errorf("length: %w", err)
		}
		return lengthNode{inner: inner, raw: arg}, nil
	case "exists":
		want, ok := arg.(bool)
		if !ok {
			return nil, fmt.Errorf("exists: needs true or false, got %s", toJSON(arg))
		}
		return existsNode{want: want}, nil
	case "json":
		return compileJSON(arg)
	}
	build, ok := leafOps[name]
	if !ok {
		return nil, fmt.Errorf("unknown operator %q (known operators: %s)", name, operatorNames())
	}
	n, err := build(name, arg, t.value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return n, nil
}

func compileList(name string, arg any, t typing) (allNode, error) {
	list, ok := arg.([]any)
	if !ok || len(list) == 0 {
		return nil, fmt.Errorf("%s: needs a non-empty list of conditions", name)
	}
	nodes := make(allNode, 0, len(list))
	for i, item := range list {
		n, err := compileCond(item, t)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", name, i, err)
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func operatorNames() string {
	names := []string{"all", "any", "not", "length", "exists", "json", "every", "some", "path"}
	for n := range leafOps {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
